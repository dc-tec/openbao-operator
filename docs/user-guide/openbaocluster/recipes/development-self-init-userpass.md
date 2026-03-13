---
description: Step-by-step recipe for a Development-profile OpenBao cluster with self-init, Operator-managed TLS, demo userpass login, and JWT admin access.
---

# Development Profile with Self-Init and Userpass

This recipe deploys a local-development `OpenBaoCluster` with:

- `spec.profile: Development`
- `spec.tls.mode: OperatorManaged`
- `spec.selfInit.enabled: true`
- `userpass` login for a demo UI user
- JWT login for a human admin `ServiceAccount`

!!! success "Based on Validated E2E Patterns"
    This recipe is built from the Development-profile lifecycle patterns exercised by the in-repo E2E suites, especially cluster lifecycle and self-init flows. The optional `userpass` demo login is a documentation convenience layered on top of those validated patterns, so keep it scoped to local development and evaluation.

!!! warning "Not for Production"
    The `Development` profile is highly discouraged for production. It relaxes core security guarantees and uses a demo `userpass` login for convenience.

## Prerequisites

- OpenBao Operator is installed.
- In multi-tenant mode, the operator can provision the target namespace through `OpenBaoTenant`.
- A StorageClass is available for the cluster PVCs.
- If your local nodes do not support AppArmor, be prepared to set `spec.workloadHardening.appArmorEnabled: false`.

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-demo` | Tenant namespace for the cluster |
| `<cluster-name>` | `openbaocluster-demo` | `OpenBaoCluster` name |
| `<openbao-version>` | `2.5.1` | OpenBao version |
| `<gateway-name>` | `traefik-gateway` | Existing passthrough-capable Gateway for optional external access |
| `<gateway-namespace>` | `default` | Namespace of the Gateway |
| `<external-host>` | `bao-demo.example.com` | External hostname for optional Gateway exposure |

## Step 1: Create the tenant namespace

Apply the tenant namespace, onboarding request, and admin `ServiceAccount`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: <namespace>
  labels:
    openbao.org/tenant: "true"
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: <cluster-name>-tenant
  namespace: openbao-operator-system
spec:
  targetNamespace: <namespace>
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openbao-admin
  namespace: <namespace>
```

Verify that the tenant is provisioned:

```bash
kubectl -n openbao-operator-system describe openbaotenant <cluster-name>-tenant
```

## Step 2: Apply the OpenBaoCluster

Apply the cluster manifest:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: <cluster-name>
  namespace: <namespace>
spec:
  profile: Development
  replicas: 3
  version: "<openbao-version>"

  tls:
    enabled: true
    mode: OperatorManaged
    rotationPeriod: "720h"

  configuration:
    logLevel: "info"
    ui: true
    logging:
      format: "json"
    defaultLeaseTTL: "720h"
    maxLeaseTTL: "8760h"
    cacheSize: 134217728
    disableCache: false
    raft:
      performanceMultiplier: 2

  storage:
    size: "10Gi"
  deletionPolicy: DeleteAll

  selfInit:
    enabled: true
    oidc:
      enabled: true
    requests:
      - name: enable-userpass-auth
        operation: update
        path: sys/auth/userpass
        authMethod:
          type: userpass
      - name: enable-jwt-auth
        operation: update
        path: sys/auth/jwt
        authMethod:
          type: jwt
      - name: enable-demo-kv
        operation: update
        path: sys/mounts/secret
        secretEngine:
          type: kv
          description: "Demo KV v2 engine"
          options:
            version: "2"
      - name: create-admin-policy
        operation: update
        path: sys/policies/acl/admin
        policy:
          policy: |
            path "*" {
              capabilities = ["create", "read", "update", "delete", "list", "sudo"]
            }
      - name: create-demo-ui-user
        operation: update
        path: auth/userpass/users/demo-admin
        data:
          password: "demo-password"
          token_policies:
            - admin
      - name: create-admin-jwt-role
        operation: update
        path: auth/jwt/role/admin
        data:
          role_type: jwt
          user_claim: sub
          bound_audiences:
            - openbao-internal
          bound_subject: system:serviceaccount:<namespace>:openbao-admin
          token_policies:
            - admin
          policies:
            - admin
          ttl: 1h
```

!!! warning "Demo-only credentials"
    The `demo-admin` user exists only to make local validation easy. Do not reuse this pattern or password in shared or production environments.

!!! note "AppArmor on local clusters"
    If kubelet rejects the Pods because AppArmor is unavailable, add:

    ```yaml
    spec:
      workloadHardening:
        appArmorEnabled: false
    ```

## Step 3: Optional Gateway exposure

If you already have a passthrough Gateway listener, add this block under `spec` before applying the manifest or patch the cluster and re-apply it:

```yaml
gateway:
  enabled: true
  listenerName: websecure-passthrough
  gatewayRef:
    name: <gateway-name>
    namespace: <gateway-namespace>
  hostname: "<external-host>"
  tlsPassthrough: true
```

If your Gateway terminates TLS at the edge instead, switch `tlsPassthrough` to `false` and enable `backendTLS`.

If you skip this step, use port-forward for verification.

## Operations

### Verify the cluster is ready

Check the status conditions:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

The steady-state expectation is:

- `Available=True`
- `TLSReady=True`
- `UserAccessBootstrap=True`
- `OpenBaoInitialized=True`
- `OpenBaoSealed=False`

`ProductionReady` is not the goal for a `Development` cluster.

If you enabled optional Gateway exposure, also expect `GatewayIntegrationReady=True`.

Confirm that no root token Secret was created:

```bash
kubectl -n <namespace> get secret <cluster-name>-root-token
```

This should return `NotFound`.

### Verify the demo UI login

Port-forward the Service:

```bash
kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"
```

Open `https://127.0.0.1:8200/ui` in a browser and sign in with:

- Username: `demo-admin`
- Password: `demo-password`

!!! note "TLS verification in local validation"
    The Development recipe uses Operator-managed certificates. Your browser or CLI may warn about a self-signed CA during local validation.

### Verify JWT admin login

Create a Kubernetes token for the admin `ServiceAccount` and exchange it for an OpenBao token:

```bash
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \
  -H 'Content-Type: application/json' \
  -d "{\"role\":\"admin\",\"jwt\":\"${JWT}\"}" \
  "${VAULT_ADDR%/}/v1/auth/jwt/login"
```

## Common Failures

- Pods fail with AppArmor errors on local clusters: set `spec.workloadHardening.appArmorEnabled: false`.
- `OpenBaoInitialized=False`: check the `selfInit.requests` paths and data for syntax errors.
- `UserAccessBootstrap=Unknown`: verify that the JWT or `userpass` bootstrap requests were applied as intended.
- A root token Secret exists: `spec.selfInit.enabled` was omitted or rejected.
- The demo UI login fails: confirm `auth/userpass` was enabled and the `demo-admin` user request was applied.

## See Also

- [Recipes Overview](index.md)
- [Scheduled Backups to S3-Compatible Storage](scheduled-backups-s3-compatible.md)
- [Security Profiles](../configuration/security-profiles.md)
- [Self-Initialization](../configuration/self-init.md)
