# Self-Initialization

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
        auditDevice:
          type: file
          fileOptions:
            filePath: stdout
      # Enable JWT auth (or use Hardened profile for automatic bootstrap)
      - name: enable-jwt-auth
        operation: update
        path: sys/auth/jwt
        authMethod:
          type: jwt
          description: "JWT authentication for Kubernetes"
      # Configure JWT auth.
      #
      # NOTE: Some Kubernetes environments require authentication for OIDC discovery
      # (/.well-known/openid-configuration). In those cases, prefer configuring JWT
      # auth using the cluster's JWKS public keys (jwt_validation_pubkeys) rather
      # than oidc_discovery_url.
      # Note: auth/jwt/config is not a sys/auth/* path, so it uses raw data.
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
| `operation` | API operation: `create`, `read`, `update`, `delete`, `list`, or `patch`. | Yes |
| `path` | API path (e.g., `sys/audit/stdout`, `sys/auth/jwt`, `sys/mounts/secret`, `sys/policies/acl/app-policy`) | Yes |
| `data` | Request payload for paths that don't have structured types (e.g., `auth/jwt/config`, `auth/kubernetes/config`). **Do not place sensitive values (tokens, passwords, unseal keys, long-lived credentials) here**, as this data is stored in the `OpenBaoCluster` resource and persisted in etcd. **For supported paths, structured fields are required** (`auditDevice`, `authMethod`, `secretEngine`, `policy`). | No |
| `auditDevice` | Structured configuration for audit devices (only for `sys/audit/*` paths). See [audit device types](#audit-device-types) below. | No |
| `authMethod` | Structured configuration for auth methods (only for `sys/auth/*` paths). See [auth method types](#auth-method-types) below. | No |
| `secretEngine` | Structured configuration for secret engines (only for `sys/mounts/*` paths). See [secret engine types](#secret-engine-types) below. | No |
| `policy` | Structured configuration for policies (only for `sys/policies/*` paths). See [policy types](#policy-types) below. | No |
| `allowFailure` | If `true`, failures don't block initialization | No |

### 7.3 Common Self-Init Patterns

**Enable KV Secrets Engine (Structured):**

```yaml
- name: enable-kv-secrets
  operation: update
  path: sys/mounts/secret
  secretEngine:
    type: kv
    description: "Key-Value secrets engine"
    options:
      version: "2"
```

**Enable Auth Method (Structured):**

```yaml
- name: enable-jwt-auth
  operation: update
  path: sys/auth/jwt
  authMethod:
    type: jwt
    description: "JWT authentication method"
```

**Create a Policy (Structured):**

```yaml
- name: create-app-policy
  operation: update
  path: sys/policies/acl/app-policy
  policy:
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

### 7.5 Audit Device Types

For audit device requests (`sys/audit/*`), use the structured `auditDevice` field instead of raw `data`:

**File Audit Device:**

```yaml
- name: enable-file-audit
  operation: update
  path: sys/audit/file
  auditDevice:
    type: file
    description: "File audit device for production logging"
    fileOptions:
      filePath: /var/log/openbao/audit.log
      mode: "0600"  # Optional, defaults to "0600"
```

**HTTP Audit Device:**

```yaml
- name: enable-http-audit
  operation: update
  path: sys/audit/http
  auditDevice:
    type: http
    httpOptions:
      uri: "https://audit.example.com/log"
      headers:
        X-Custom-Header: "value"
```

**Syslog Audit Device:**

```yaml
- name: enable-syslog-audit
  operation: update
  path: sys/audit/syslog
  auditDevice:
    type: syslog
    syslogOptions:
      facility: "AUTH"
      tag: "openbao"
```

**Socket Audit Device:**

```yaml
- name: enable-socket-audit
  operation: update
  path: sys/audit/socket
  auditDevice:
    type: socket
    socketOptions:
      address: "127.0.0.1:9000"
      socketType: "tcp"
      writeTimeout: "2s"
```

### 7.6 Auth Method Types

For auth method requests (`sys/auth/*`), use the structured `authMethod` field:

```yaml
- name: enable-jwt-auth
  operation: update
  path: sys/auth/jwt
  authMethod:
    type: jwt
    description: "JWT authentication for Kubernetes"
    config:
      default_lease_ttl: "1h"
      max_lease_ttl: "24h"
```

Common auth method types include: `jwt`, `kubernetes`, `userpass`, `ldap`, `oidc`, `cert`.

### 7.7 Secret Engine Types

For secret engine requests (`sys/mounts/*`), use the structured `secretEngine` field:

```yaml
- name: enable-kv-secrets
  operation: update
  path: sys/mounts/secret
  secretEngine:
    type: kv
    description: "Key-Value secrets engine"
    options:
      version: "2"
```

Common secret engine types include: `kv`, `pki`, `transit`, `database`, `aws`, `azure`, `gcp`.

### 7.8 Policy Types

For policy requests (`sys/policies/*`), use the structured `policy` field:

```yaml
- name: create-app-policy
  operation: update
  path: sys/policies/acl/app-policy
  policy:
    policy: |
      path "secret/data/app/*" {
        capabilities = ["read", "list"]
      }
      path "secret/metadata/app/*" {
        capabilities = ["list"]
      }
```

The policy content can be written in HCL or JSON format.

Check the status to see if self-initialization completed:

```sh
kubectl -n security get openbaocluster self-init-cluster -o jsonpath='{.status.selfInitialized}'
# Returns: true
```
