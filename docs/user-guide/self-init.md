# 7. Self-Initialization

[Back to User Guide index](README.md)

OpenBao supports [self-initialization](https://openbao.org/docs/configuration/self-init/), which allows you to declaratively configure your cluster during first startup. When enabled, OpenBao automatically:

- Initializes itself using auto-unseal
- Executes configured API requests (audit devices, auth methods, secret engines, policies)
- Revokes the root token after initialization completes

**Important:** When self-initialization is enabled, no root token Secret is created. The root token is automatically revoked by OpenBao after the init requests complete.

### 7.1 Basic Self-Initialization Example

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: self-init-cluster
  namespace: security
spec:
  version: "2.4.0"
  image: "openbao/openbao:2.4.0"
  replicas: 3
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
  selfInit:
    enabled: true
    requests:
      # Enable stdout audit logging first (recommended for debugging)
      - name: enable-stdout-audit
        operation: update
        path: sys/audit/stdout
        data:
          type: file
          options:
            file_path: /dev/stdout
            log_raw: true
      # Enable JWT auth (or use Hardened profile for automatic bootstrap)
      - name: enable-jwt-auth
        operation: update
        path: sys/auth/jwt
        data:
          type: jwt
      # Configure JWT auth.
      #
      # NOTE: Some Kubernetes environments require authentication for OIDC discovery
      # (/.well-known/openid-configuration). In those cases, prefer configuring JWT
      # auth using the cluster's JWKS public keys (jwt_validation_pubkeys) rather
      # than oidc_discovery_url.
      - name: configure-jwt-auth
        operation: update
        path: auth/jwt/config
        data:
          bound_issuer: "https://kubernetes.default.svc"
          jwt_validation_pubkeys:
            - "<K8S_JWT_PUBLIC_KEY_PEM>"
  deletionPolicy: Retain
```

### 7.2 Self-Init Request Structure

Each request in `spec.selfInit.requests[]` maps to an OpenBao API call:

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Unique identifier (must match `^[A-Za-z_][A-Za-z0-9_-]*$`) | Yes |
| `operation` | API operation: `create`, `read`, `update`, `delete`, `list` | Yes |
| `path` | API path (e.g., `sys/audit/stdout`, `auth/jwt/config`) | Yes |
| `data` | Request payload (structure depends on the API endpoint). **Do not place sensitive values (tokens, passwords, unseal keys, long-lived credentials) here**, as this data is stored in the `OpenBaoCluster` resource and persisted in etcd. | No |
| `allowFailure` | If `true`, failures don't block initialization | No |

### 7.3 Common Self-Init Patterns

**Enable KV Secrets Engine:**

```yaml
- name: enable-kv-secrets
  operation: update
  path: sys/mounts/secret
  data:
    type: kv
    options:
      version: "2"
```

**Create a Policy:**

```yaml
- name: create-app-policy
  operation: update
  path: sys/policies/acl/app-policy
  data:
    policy: |
      path "secret/data/app/*" {
        capabilities = ["read", "list"]
      }
```

### 7.4 Standard vs Self-Initialization

| Aspect | Standard Init | Self-Init |
|--------|---------------|-----------|
| Root Token | Stored in `<cluster>-root-token` Secret | Auto-revoked, not available |
| Initial Config | Manual post-init via CLI/API | Declarative in CR |
| Recovery | Root token available for recovery | Must use other auth methods |
| Use Case | Dev/test, manual management | Production, GitOps workflows |

Check the status to see if self-initialization completed:

```sh
kubectl -n security get openbaocluster self-init-cluster -o jsonpath='{.status.selfInitialized}'
# Returns: true
```
