# HLD/LLD: Inline Authentication for OpenBao Operator

Date: 2026-05-22
Status: Draft
Authors: Roel de Cort / OpenBao Operator maintainers

## Purpose

Design the operator-side adoption of OpenBao inline authentication for
JWT-backed controller and executor requests.

The design goal is narrow: replace the current "login with projected
ServiceAccount JWT, receive an OpenBao client token, then send
`X-Vault-Token`" runtime path with OpenBao inline authentication for supported
operator-owned API calls.

This is a workstream design note, not user-facing documentation.

## Source Facts

OpenBao inline authentication was implemented upstream in
`openbao/openbao#1433`, merged on 2025-06-17, and released in OpenBao v2.4.0 on
2025-08-28.

The operator currently validates against OpenBao 2.4.4 and 2.5.4. Because the
supported floor is at least 2.4.4, every supported managed OpenBao version has
inline authentication support.

Inline authentication is documented as a request-header based flow:

- `X-Vault-Inline-Auth-Path`
- optional `X-Vault-Inline-Auth-Operation`, defaulting to `update`
- optional `X-Vault-Inline-Auth-Namespace`
- one or more `X-Vault-Inline-Auth-Parameter-*` headers containing URL-safe
  base64, without padding, JSON objects with `key` and `value`

The resulting ephemeral token is not returned to the client and is not
persisted to storage. Inline auth does not support operations that create
leases.

## Current Operator Baseline

The operator already has a good identity model:

- controller and executor identities use projected Kubernetes ServiceAccount
  JWTs
- OpenBao roles bind those JWTs through `auth/jwt-operator`
- backup, restore, and upgrade each use their own role and ServiceAccount
- `tokenSecretRef` remains a fallback for selected token-based flows

The runtime transport is still standard authentication.

### Controller day-2 operations

The controller reads a projected token from
`/var/run/secrets/tokens/openbao-token` and builds an authenticated OpenBao
client through `ClientFactory.NewWithJWT(...)`.

Relevant code:

- `internal/adapter/raft/autopilot.go`
- `internal/service/init/manager.go`
- `internal/adapter/openbao/factory.go`
- `internal/adapter/openbao/client_bootstrap.go`

The factory currently performs a `POST /v1/auth/jwt-operator/login`, caches the
returned OpenBao client token, and constructs a client that sets
`X-Vault-Token`.

### Backup and restore executor

The backup binary is also used for restore. It loads `BACKUP_AUTH_METHOD`,
`BACKUP_JWT_AUTH_ROLE`, and the projected JWT token file. For JWT auth it logs
in first, then opens a token-backed OpenBao client.

Relevant code:

- `cmd/bao-backup/openbao_runtime.go`
- `internal/service/backup/config_tls_auth.go`
- `internal/service/backup/jobenv/build.go`
- `internal/service/restore/job.go`

### Upgrade executor

The upgrade executor loads `UPGRADE_JWT_AUTH_ROLE` and the projected JWT token.
The raft operation helpers call `LoginJWT(...)`, then build token-backed
clients for leader step-down, join, raft configuration, promote, demote, and
remove-peer operations.

Relevant code:

- `internal/service/upgrade/raftops/config.go`
- `internal/service/upgrade/raftops/leader_search.go`
- `internal/service/upgrade/raftops/actions.go`
- `internal/service/upgrade/raftops/leader_transfer.go`

### OpenBao client transport

The OpenBao client stores a token string on `Client` and most authenticated
methods manually set `X-Vault-Token`.

Relevant code:

- `internal/adapter/openbao/client.go`
- `internal/adapter/openbao/factory.go`
- `internal/adapter/openbao/client_bootstrap.go`
- `internal/adapter/openbao/client_health.go`
- `internal/adapter/openbao/client_raft.go`

The existing `jwtTokenCache` only exists to reduce repeated standard login
requests. Inline auth makes this cache unnecessary for the default JWT path,
but it should stay while standard JWT fallback remains available.

## Problem

Standard JWT authentication is now an avoidable intermediate token flow for the
operator's supported OpenBao versions.

The current flow has three costs:

1. It performs an extra OpenBao login request before useful work.
2. It makes OpenBao persist token and expiration-manager state for short-lived
   operator work.
3. It makes the operator receive, cache, pass, and test OpenBao client tokens
   that are not part of the user-facing identity contract.

This matters most for short-lived executor jobs. The controller already caches
standard JWT login tokens, so its steady-state login pressure is lower, but the
token handling is still unnecessary.

## Goals

- Use inline authentication by default for JWT-backed operator-to-OpenBao calls.
- Preserve the existing CRD contract: `jwtAuthRole` still means "authenticate
  with the projected ServiceAccount JWT and this OpenBao role".
- Preserve `tokenSecretRef` and root-token flows as token-backed standard auth.
- Avoid OpenBao version probing for this feature because the supported floor is
  already high enough.
- Centralize auth header construction so individual OpenBao API methods do not
  hand-roll auth behavior.
- Keep a temporary standard-JWT fallback for operational compatibility with
  proxies, gateways, mocks, and debugging.
- Keep all existing role names, policies, ServiceAccounts, and projected-token
  audience behavior unchanged.

## Non-Goals

- No CRD API changes.
- No new OpenBao role or policy model.
- No switch from JWT auth to Kubernetes auth.
- No support for leased secret engine operations through inline auth.
- No removal of `tokenSecretRef`.
- No removal of standard JWT login support in the first implementation.
- No attempt to solve human login or user-facing authentication.

## Design Decision

Use inline authentication as the default transport for `jwtAuthRole` flows.

Token-backed flows continue to use `X-Vault-Token`:

- non-self-init root-token fallback
- backup and restore `tokenSecretRef`
- any future explicit token auth path

JWT-backed flows use inline authentication:

- controller day-2 raft and autopilot operations
- backup snapshot jobs
- restore snapshot-force jobs
- rolling upgrade raft operations
- blue/green upgrade raft operations

The public contract does not expose "inline auth" as a CRD field. Inline auth is
an implementation detail of the operator's supported OpenBao transport.

## Request Auth Model

Introduce an internal request authorization layer in `internal/adapter/openbao`.

Suggested shape:

```go
type requestAuthorizer interface {
	Authorize(req *http.Request) error
	RequiresAuth() bool
	Kind() string
}
```

Concrete implementations:

- `noAuthAuthorizer`
- `tokenAuthorizer`
- `inlineJWTAuthorizer`

The `Client` should hold an authorizer instead of treating `token string` as
the only auth state.

```go
type Client struct {
	baseURL    string
	httpClient *http.Client
	state      *clientState
	auth       requestAuthorizer
}
```

The first implementation can keep `token string` for compatibility if that
reduces churn, but request auth should be applied through one helper:

```go
func (c *Client) authorize(req *http.Request) error
```

OpenBao API methods then call `c.authorize(req)` instead of setting
`X-Vault-Token` directly.

## Inline JWT Header Construction

For the operator's existing auth mount, inline JWT auth should generate:

```text
X-Vault-Inline-Auth-Path: jwt-operator/login
X-Vault-Inline-Auth-Operation: update
X-Vault-Inline-Auth-Parameter-role: <base64url-no-padding({"key":"role","value":"<role>"})>
X-Vault-Inline-Auth-Parameter-jwt: <base64url-no-padding({"key":"jwt","value":"<jwt>"})>
```

The parameter header suffixes should be deterministic and non-sensitive. The
secret value lives in the encoded header value, not the suffix.

Rules:

- never set `X-Vault-Token` on an inline-auth request
- reject client construction when inline JWT auth has an empty role or JWT
- trim role and JWT whitespace before storing them in the authorizer
- never include JWT values in errors or logs
- make header construction independently unit tested

## Auth Strategy Fallback

Even though OpenBao version compatibility is no longer a blocker, an
installation-scoped fallback is still useful because inline auth relies on
custom headers reaching OpenBao intact.

Add an internal auth strategy setting:

```text
OPENBAO_JWT_AUTH_STRATEGY=inline|standard
```

Default: `inline`.

Behavior:

- `inline` uses inline JWT authentication for `jwtAuthRole` flows
- `standard` preserves the current login-plus-token path
- invalid values fail fast during controller or executor startup/config load

This is intentionally not a CRD field. It is an installation/runtime escape
hatch, not a per-cluster service contract.

The controller must propagate the selected strategy into backup, restore, and
upgrade jobs so executor behavior matches the installed operator posture.

## Factory API

Add explicit constructors while keeping the existing standard auth path:

```go
func (f *ClientFactory) NewWithToken(baseURL, token string) (*Client, error)
func (f *ClientFactory) NewWithInlineJWT(baseURL, role, jwtToken string) (*Client, error)
func (f *ClientFactory) NewWithStandardJWT(ctx context.Context, baseURL, role, jwtToken string) (*Client, error)
```

Transition options:

1. Keep `NewWithJWT(...)` as the standard-login method and update call sites to
   choose `NewWithInlineJWT(...)` explicitly.
2. Change `NewWithJWT(...)` to mean "JWT using configured strategy" and add a
   separately named standard-login method.

Preferred first implementation: option 1.

Reasoning:

- it limits semantic surprise in existing tests
- it preserves a direct standard-login method for fallback
- it makes the inline migration visible at the call sites

After the rollout is stable, a cleanup can rename the factory methods around
the new default model.

## Authenticated Operation Inventory

The current operator-owned authenticated operations are non-leased and are
eligible for inline auth.

Controller:

- `GET /v1/sys/health`
- `PUT /v1/sys/step-down`
- `GET /v1/sys/storage/raft/configuration`
- `GET /v1/sys/storage/raft/autopilot/state`
- `POST /v1/sys/storage/raft/autopilot/configuration`
- `POST /v1/sys/storage/raft/remove-peer`

Backup and restore:

- `GET /v1/sys/storage/raft/snapshot`
- `POST /v1/sys/storage/raft/snapshot-force`

Upgrade:

- `GET /v1/sys/health`
- `PUT /v1/sys/step-down`
- `PUT /v1/sys/storage/raft/join`
- `GET /v1/sys/storage/raft/configuration`
- `PUT /v1/sys/storage/raft/configuration`
- `POST /v1/sys/storage/raft/remove-peer`
- `POST /v1/sys/storage/raft/promote`
- `POST /v1/sys/storage/raft/demote`
- `GET /v1/sys/storage/raft/autopilot/state`

Unauthenticated leader discovery can remain unauthenticated. If an already
authenticated client calls health, the auth layer can still attach inline auth
to preserve the existing "token if present" behavior.

Future code must not use inline auth for dynamic secrets, certificate issuance,
or any request that returns a lease unless the OpenBao behavior changes.

## Component Changes

### `internal/adapter/openbao`

Add:

- inline auth constants
- header encoder
- authorizer implementations
- tests for token, inline JWT, and unauthenticated requests

Refactor:

- replace direct `req.Header.Set("X-Vault-Token", c.token)` calls with
  `c.authorize(req)`
- keep standard JWT login code for fallback
- keep `jwtTokenCache` only on the standard JWT path

### `internal/adapter/raft`

Change controller self-init JWT client construction:

- read projected token exactly as today
- choose inline or standard strategy
- create an OpenBao client through the selected factory method

No role, policy, or ServiceAccount changes.

### `cmd/bao-backup`

Change `authenticate(...)` and `openClusterClient(...)` so JWT auth does not
return an OpenBao token in the default path.

Suggested split:

```go
func openClusterClient(ctx context.Context, cfg *ExecutorConfig, purpose, leaderURL string) (portopenbao.ClusterActions, func(), error)
```

Inside:

- if auth method is JWT and strategy is inline, use `NewWithInlineJWT`
- if auth method is JWT and strategy is standard, use the existing login flow
- if auth method is token, use `NewWithToken`

### `internal/service/upgrade/raftops`

Replace per-operation `LoginJWT(...)` plus `NewWithToken(...)` with a helper
that returns an authenticated client for the chosen strategy.

Suggested helper:

```go
func NewAuthenticatedClient(ctx context.Context, cfg *ExecutorConfig, factory *openbao.ClientFactory, baseURL string) (*openbao.Client, error)
```

This avoids repeating strategy branches across step-down, join, promote, demote,
configuration, and leader-transfer paths.

### Job builders and config loaders

Add an env constant:

```go
EnvOpenBaoJWTAuthStrategy = "OPENBAO_JWT_AUTH_STRATEGY"
```

Add config fields:

- backup executor config: `JWTAuthStrategy string`
- upgrade executor config: `JWTAuthStrategy string`

Job builders should inject the controller's strategy for JWT-backed jobs. Token
jobs do not need it, but passing it is harmless if validation ignores it outside
JWT auth mode.

## Security Considerations

Inline auth improves the operator's exposure to OpenBao client tokens: the
operator no longer receives a reusable OpenBao token for default JWT flows.

It does not remove the projected Kubernetes JWT from memory or headers. The JWT
is still sensitive and must be treated like a credential.

Security rules:

- do not log inline auth header values
- do not include JWTs in wrapped errors
- do not add request dumps that include headers
- ensure inline and token auth are mutually exclusive
- keep projected token volume permissions unchanged
- preserve audience and bound-subject validation

Audit behavior changes:

- OpenBao audits both the inline authentication and the main request.
- Operators should expect more auth audit events than cached standard JWT login
  produced for long-running controller clients.
- This is acceptable because the audit trail is more precise for each request.

## Compatibility Considerations

OpenBao version:

- no version probe is required because supported OpenBao versions are >= 2.4.4
- inline auth was introduced in 2.4.0

Network path:

- custom headers must survive any Service, Gateway, Ingress, or proxy path
- large JWTs can hit intermediary header-size limits
- the `standard` strategy escape hatch covers this class of rollout issue

Mocks and tests:

- existing tests that model `/auth/jwt-operator/login` should be kept for the
  standard strategy
- new tests should assert the inline default path avoids login

HashiCorp Vault compatibility is not a goal for managed clusters. The operator
targets OpenBao.

## Rollout Plan

### Phase 1: Transport primitives

- Add request authorizers and inline header encoder.
- Refactor OpenBao client methods to use centralized auth.
- Preserve all standard JWT login behavior.
- Add focused unit coverage.

### Phase 2: Controller path

- Add strategy config to controller runtime.
- Use inline auth by default for controller raft/autopilot clients.
- Keep `OPENBAO_JWT_AUTH_STRATEGY=standard` fallback.
- Update controller tests and mock contracts.

### Phase 3: Backup and restore executor path

- Add strategy config loading to the backup binary.
- Change JWT-backed backup and restore clients to inline by default.
- Preserve token mode unchanged.
- Update backup/restore unit tests so the default JWT path hits snapshot or
  restore directly with inline headers and no login request.

### Phase 4: Upgrade executor path

- Add strategy config loading to upgrade executor config.
- Replace repeated login-token construction with one authenticated-client
  helper.
- Update rolling and blue/green raftops tests.

### Phase 5: Documentation and cleanup

- Update operator auth docs to mention inline auth as the implementation detail
  for supported OpenBao versions.
- Document the installation-scoped fallback env var.
- Leave CRD docs unchanged unless they currently promise standard login tokens.
- Decide whether to rename `NewWithJWT` after the migration settles.

## Test Plan

### Unit tests

OpenBao client:

- inline header encoder emits URL-safe base64 without padding
- encoder preserves `role` and `jwt` values after decode
- inline JWT authorizer sets the four expected inline headers
- inline JWT authorizer does not set `X-Vault-Token`
- token authorizer only sets `X-Vault-Token`
- client construction fails on empty role or empty JWT for inline auth
- request authorizer is applied to snapshot, restore, raft, autopilot, and
  step-down requests

Factory:

- `NewWithInlineJWT` does not call `/auth/jwt-operator/login`
- `NewWithStandardJWT` still calls `/auth/jwt-operator/login`
- standard JWT cache behavior remains covered

Executors:

- backup JWT default uses inline headers on snapshot request
- restore JWT default uses inline headers on snapshot-force request
- token backup/restore keeps `X-Vault-Token`
- upgrade JWT default uses inline headers for leader step-down and raft actions
- `OPENBAO_JWT_AUTH_STRATEGY=standard` preserves current login behavior
- invalid strategy fails config loading

### Integration tests

- self-init OIDC cluster reaches ready state with controller inline auth
- scheduled or manual backup succeeds with inline auth
- restore succeeds with inline auth
- rolling upgrade succeeds with inline auth
- blue/green upgrade succeeds with inline auth
- fallback strategy succeeds on at least one representative backup or upgrade
  path

### Manual validation

Use OpenBao 2.4.4 and 2.5.4:

- verify no token-store login entries are created for inline-auth executor calls
- verify audit logs contain both inline auth and main request entries
- verify proxy/Gateway paths preserve inline auth headers in the validated
  deployment lanes

## Acceptance Criteria

- Default JWT-backed operator paths no longer perform
  `/v1/auth/jwt-operator/login`.
- Default JWT-backed operator requests do not set `X-Vault-Token`.
- Default JWT-backed operator requests set valid inline auth headers.
- Token-backed flows are unchanged.
- No CRD schema changes are required.
- Existing JWT role, policy, ServiceAccount, and audience contracts remain
  unchanged.
- All PR-equivalent local checks that cover touched packages pass.

## Open Questions

1. Should authenticated health requests attach inline auth, or should health
   stay unauthenticated even when the client has auth configured?

   Current behavior attaches `X-Vault-Token` when a token is present, so the
   least surprising first implementation is to attach inline auth for
   authenticated clients and leave unauthenticated leader discovery unchanged.

2. Should the fallback env var be documented as stable?

   Initial recommendation: document it as an operational compatibility switch,
   but do not make it a long-term API promise until after one release.

3. Should `NewWithJWT` be renamed in the same PR?

   Initial recommendation: no. Add explicit inline and standard constructors
   first, then clean naming after behavior is stable.

4. Should status expose which auth transport is active?

   Initial recommendation: no. The identity contract is unchanged and status
   should not grow a transport implementation detail.

## Work Breakdown

### IA-1: OpenBao client auth abstraction

Deliverables:

- request authorizer interface
- token and inline JWT authorizers
- inline auth header encoder
- refactored client request methods
- unit coverage

### IA-2: Controller inline auth

Deliverables:

- controller strategy config
- controller raft/autopilot client update
- fallback to standard JWT login
- controller tests

### IA-3: Backup and restore inline auth

Deliverables:

- backup executor strategy config
- job env propagation
- backup and restore client construction update
- unit and integration coverage

### IA-4: Upgrade inline auth

Deliverables:

- upgrade executor strategy config
- shared authenticated-client helper
- rolling and blue/green raftops updates
- unit and integration coverage

### IA-5: Documentation and release notes

Deliverables:

- operator auth docs update
- compatibility note that inline auth is available across the supported
  OpenBao floor
- fallback env var note
- release note entry
