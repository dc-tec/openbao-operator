---
trigger: always_on
glob: "{api,cmd,internal}/**/*.go"
description: Layered architecture and dependency direction rules for the OpenBao Operator
---

# Architecture Rules

See [Architecture Documentation](docs/architecture/index.md).

## Layer Model (L0-L7)

- `L0` API types: `api/v1alpha1`
- `L1` Entrypoints/bootstrap: `cmd/*`, `internal/platform/entrypoint`
- `L2` Controller plumbing: `internal/controller/*`
- `L3` App orchestration: `internal/app/*`
- `L4` Services/managers: `internal/service/{backup,restore,upgrade,infra,certs,init,provisioner,opslifecycle}`
- `L5` Ports/contracts: `internal/port/*`
- `L6` Adapters/integrations: `internal/adapter/{kube,openbao,storage,auth,raft,security,storageenv,cluster,config,operationlock,probe,revision}`
- `L7` Platform/cross-cutting: `internal/platform/{errors,logging,reconcile,constants,predicates,observability,admission}`

## Separation of Concerns

### Controllers (`internal/controller/*`, L2)

Reconcilers should do only:

1. Fetch input state from the API
2. Observe and assemble reconcile context
3. Delegate orchestration to `internal/app/*` and focused helpers
4. Apply final patch/result handling

Do not embed broad business orchestration directly in controllers.
Controller-local dependency adapters are allowed for import-surface management, but they must not become a business-logic layer.

### App Orchestration (`internal/app/*`, L3)

App packages coordinate domain workflows and phase sequencing.

App packages may call services/managers, ports, and platform utilities.

### Services/Managers (`internal/service/*`, L4)

Services implement domain behavior and should consume adapters through `internal/port/*` contracts.
Ports stay contract-only: they may contain interfaces, neutral types, and domain helpers, but they must not import concrete adapter packages.

Services must not import controller packages.

### Adapters (`internal/adapter/*`, L6)

Adapters implement ports and integration logic.

Adapters must not import controllers or service/manager packages.

### API Types (`api/v1alpha1`, L0)

CRD structs are data containers:

- Keep logic minimal and data-oriented
- Avoid external service behavior in API types

## Manager Pattern

Each domain manager should:

- Own its domain behavior
- Expose focused methods with explicit inputs/outputs
- Depend on narrow ports/interfaces where external dependencies are needed

## No God Objects

Do NOT pass entire Reconciler to helpers:

```go
// Bad
func helper(r *Reconciler, cluster *v1alpha1.OpenBaoCluster)

// Good
func helper(ctx context.Context, client client.Client, cluster *v1alpha1.OpenBaoCluster)
```

## Dependency Direction (Strict)

Allowed direction:

- `L1 -> L2/L3/L7`
- `L2 -> L3/L7/(small focused L4 usage when unavoidable)`
- `L3 -> L4/L5/L7`
- `L4 -> L5/L7`
- `L6 -> L5/L7`
- Any layer may consume `L0` types as needed

Disallowed direction:

- `L4/L6 -> L2` (service/adapter importing controllers)
- `L6 -> L4` (adapter importing services/managers)
- Re-introducing `internal/interfaces` as a generic dependency bucket

## Verification

For architecture-impacting changes, run:

```sh
make generate-ast-rules
make verify-arch-policy
make test-ast
make lint-ast
make report-internal-deps
```

Review:

- `dist/architecture/internal-dependency-report.md`
- `hack/architecture/dependency-policy-exceptions.tsv`
- `.ast-grep/policy/architecture-boundaries.yml`

Expectation:

- Keep dependency policy warnings at `None` unless adding a deliberate, documented exception.
- Keep ast-grep architecture and reconcile guardrails green.
