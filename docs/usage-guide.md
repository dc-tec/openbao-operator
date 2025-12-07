# OpenBao Supervisor Operator – Usage Guide

This guide shows how to deploy the Operator, create an `OpenBaoCluster`, and understand the
core resources it manages.

See also:
- `docs/high-level-design.md`
- `docs/technical-design-document.md`
- `docs/implementation-plan.md`

## 1. Deploy the Operator

Build and push an image:

```sh
make docker-build docker-push IMG=<your-registry>/openbao-operator:tag
```

Install CRDs and deploy the manager:

```sh
make install
make deploy IMG=<your-registry>/openbao-operator:tag
```

Verify the controller is running:

```sh
kubectl -n openbao-operator-system get pods
```

Adjust the namespace above if you changed the default deployment namespace.

## 2. Create a Basic OpenBaoCluster

The minimal spec focuses on version, image, replicas, TLS, and storage:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 3
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
  deletionPolicy: Retain
```

Apply the resource:

```sh
kubectl apply -f dev-cluster.yaml
```

Check the CR status:

```sh
kubectl -n security get openbaoclusters.openbao.org dev-cluster -o yaml
```

Look for:

- `status.phase`
- `status.readyReplicas`
- `status.initialized` (becomes `true` after the cluster has been initialized)
- `status.conditions` (especially `Available`, `TLSReady`, `Degraded`, `EtcdEncryptionWarning`)

## 3. What the Operator Creates

For a cluster named `dev-cluster` in namespace `security`, the Operator reconciles:

- TLS Secrets (when `spec.tls.mode` is `OperatorManaged` or omitted):
  - `dev-cluster-tls-ca` – Root CA (`ca.crt`, `ca.key`). Created and managed by the operator.
  - `dev-cluster-tls-server` – server cert/key and CA bundle (`tls.crt`, `tls.key`, `ca.crt`). Created and managed by the operator.
- TLS Secrets (when `spec.tls.mode` is `External`):
  - `dev-cluster-tls-ca` – CA certificate (`ca.crt`, optionally `ca.key`). Must be created by an external provider (e.g., cert-manager).
  - `dev-cluster-tls-server` – server cert/key and CA bundle (`tls.crt`, `tls.key`, `ca.crt`). Must be created by an external provider. The operator waits for these Secrets and triggers hot-reloads when they change.
- Auto-unseal Secret (only when using static seal):
  - `dev-cluster-unseal-key` – raw 32-byte unseal key (created only if `spec.unseal` is omitted or `spec.unseal.type` is `"static"`).
- Root token Secret (created during initialization):
  - `dev-cluster-root-token` – initial root token (`token` key). See Security section below.
- ConfigMap:
  - `dev-cluster-config` – rendered `config.hcl` containing:
    - `listener "tcp"` with TLS paths under `/etc/bao/tls`.
    - `seal` stanza (type depends on `spec.unseal.type`):
      - `seal "static"` pointing at `/etc/bao/unseal/key` (default).
      - `seal "awskms"`, `seal "gcpckms"`, `seal "azurekeyvault"`, or `seal "transit"` with provider-specific options (when `spec.unseal.type` is set).
    - `storage "raft"` with `path = "/bao/data"` and:
      - a bootstrap `retry_join` targeting pod-0 for initial cluster formation.
      - a post-initialization `retry_join` using `auto_join = "provider=k8s namespace=<ns> label_selector=\"openbao.org/cluster=<name>\""` so additional pods can join dynamically.
    - Any additional non-protected attributes from `spec.config` as string key/value pairs (validated via allowlist).
    - Audit device blocks from `spec.audit` (if configured).
    - Plugin blocks from `spec.plugins` (if configured).
    - Telemetry block from `spec.telemetry` (if configured).
- NetworkPolicy:
  - `dev-cluster-network-policy` – automatically created to enforce cluster isolation:
    - Default deny all ingress traffic.
    - Allow ingress from pods within the same cluster (same pod selector labels).
    - Allow ingress from kube-system namespace (for DNS and system components).
    - Allow ingress from OpenBao operator pods on port 8200 (for health checks, initialization, upgrades).
    - Allow egress to DNS (port 53 UDP/TCP) for service discovery.
    - Allow egress to Kubernetes API server (port 443) for pod discovery (discover-k8s provider).
    - Allow egress to cluster pods on ports 8200 and 8201 for Raft communication.
    - **Note:** Backup job pods (labeled with `openbao.org/component=backup`) are excluded from this NetworkPolicy to allow access to object storage. Users can create custom NetworkPolicies for backup jobs if additional restrictions are needed.
- Services:
  - Headless Service `dev-cluster` (ClusterIP `None`) backing the StatefulSet.
  - Optional external Service `dev-cluster-public` when `spec.service` or `spec.ingress.enabled` is set.
- StatefulSet:
  - `dev-cluster` with:
    - `replicas = spec.replicas`.
    - PVC template `data` sized from `spec.storage.size`.
    - Volumes:
      - TLS Secret at `/etc/bao/tls`.
      - ConfigMap at `/etc/bao/config`.
      - Unseal Secret at `/etc/bao/unseal/key` (only when using static seal).
      - KMS credentials at `/etc/bao/seal-creds` (when using external KMS with `spec.unseal.credentialsSecretRef`).
      - Data at `/bao/data`.
    - HTTPS liveness/readiness probes on `/v1/sys/health` (port 8200).

You can inspect these resources with:

```sh
kubectl -n security get statefulsets,svc,cm,secrets -l openbao.org/cluster=dev-cluster
```

## 4. External Access (Service and Ingress)

To expose OpenBao externally, configure `spec.service` and/or `spec.ingress`:

```yaml
spec:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
  ingress:
    enabled: true
    host: "bao.example.com"
    path: "/"
    annotations:
      nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
```

With this configuration:

- The Operator ensures `dev-cluster-public` exists as an external Service.
- An `Ingress` named `dev-cluster` routes `https://bao.example.com/` to `dev-cluster-public`.
- The TLS Secret for the Ingress defaults to `dev-cluster-tls-server` unless `tlsSecretName` is set.
- The `bao.example.com` host is automatically added to the server certificate SANs.
- The external service DNS name (`{cluster-name}-public.{namespace}.svc`) is automatically added to the server certificate SANs when service or ingress is configured.

Always validate connectivity through your ingress or load balancer after the cluster reports
`Available=True`.

### 4.1 Traefik v3 Configuration

When using Traefik v3 with TLS termination at the ingress and HTTPS backend communication, you need to configure Traefik to trust the OpenBaoCluster CA certificate:

1. **Create a ServersTransport** resource that references the existing CA secret (see `config/samples/traefik-servers-transport.yaml`):
   ```yaml
   apiVersion: traefik.io/v1alpha1
   kind: ServersTransport
   metadata:
     name: openbao-tls-transport
     namespace: <your-namespace>
   spec:
     rootCAsSecrets:
       - <cluster-name>-tls-ca  # e.g., openbaocluster-external-tls-ca
   ```
   The operator automatically creates the CA secret named `<cluster-name>-tls-ca`. Traefik will read the `ca.crt` key from this secret directly - no mounting required!

2. **Reference the transport** in your OpenBaoCluster service annotations (already included in the sample):
   ```yaml
   service:
     annotations:
       traefik.ingress.kubernetes.io/service.serversscheme: https
       traefik.ingress.kubernetes.io/service.serverstransport: openbao-tls-transport
   ```
   Note: If the ServersTransport is in the same namespace as the Service, use just the name. For cross-namespace references, use `<namespace>-<name>@kubernetescrd` and ensure `allowCrossNamespace` is enabled in Traefik.

This ensures Traefik validates the OpenBao backend certificate using the cluster's CA, maintaining secure end-to-end TLS.

### 4.2 External TLS Provider (cert-manager)

For production environments that require integration with corporate PKI or cert-manager, you can configure the operator to use externally-managed TLS certificates.

Set `spec.tls.mode` to `External`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.4.4"
  image: "openbao/openbao:2.4.4"
  replicas: 3
  tls:
    enabled: true
    mode: External  # Use external certificate management
  storage:
    size: "10Gi"
```

**Important:** When using External mode, you must create the TLS Secrets before the operator can proceed. The operator expects:

- `<cluster-name>-tls-ca` - CA certificate Secret with keys `ca.crt` and optionally `ca.key`
- `<cluster-name>-tls-server` - Server certificate Secret with keys `tls.crt`, `tls.key`, and `ca.crt`

**Example with cert-manager:**

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: prod-cluster-server
  namespace: security
spec:
  secretName: prod-cluster-tls-server  # Must match operator expectation
  dnsNames:
    - prod-cluster.security.svc
    - *.prod-cluster.security.svc
    - prod-cluster-public.security.svc
  issuerRef:
    name: corporate-ca-issuer
    kind: ClusterIssuer
```

The operator will:
- Wait for the Secrets to be created (no error if missing)
- Monitor certificate changes and trigger hot-reloads when cert-manager rotates certificates
- Not attempt to generate or rotate certificates itself

**Note:** The `rotationPeriod` field is ignored in External mode, as certificate rotation is handled by the external provider.

## 5. Self-Initialization

OpenBao supports [self-initialization](https://openbao.org/docs/configuration/self-init/), which allows you to declaratively configure your cluster during first startup. When enabled, OpenBao automatically:

- Initializes itself using auto-unseal
- Executes configured API requests (audit devices, auth methods, secret engines, policies)
- Revokes the root token after initialization completes

**Important:** When self-initialization is enabled, no root token Secret is created. The root token is automatically revoked by OpenBao after the init requests complete.

### 5.1 Basic Self-Initialization Example

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
      # Enable Kubernetes auth
      - name: enable-kubernetes-auth
        operation: update
        path: sys/auth/kubernetes
        data:
          type: kubernetes
      # Configure Kubernetes auth
      - name: configure-kubernetes-auth
        operation: update
        path: auth/kubernetes/config
        data:
          kubernetes_host: "https://kubernetes.default.svc"
  deletionPolicy: Retain
```

### 5.2 Self-Init Request Structure

Each request in `spec.selfInit.requests[]` maps to an OpenBao API call:

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Unique identifier (must match `^[A-Za-z_][A-Za-z0-9_-]*$`) | Yes |
| `operation` | API operation: `create`, `read`, `update`, `delete`, `list` | Yes |
| `path` | API path (e.g., `sys/audit/stdout`, `auth/kubernetes/config`) | Yes |
| `data` | Request payload (structure depends on the API endpoint). **Do not place sensitive values (tokens, passwords, unseal keys, long-lived credentials) here**, as this data is stored in the `OpenBaoCluster` resource and persisted in etcd. | No |
| `allowFailure` | If `true`, failures don't block initialization | No |

### 5.3 Common Self-Init Patterns

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

### 5.4 Standard vs Self-Initialization

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

## 6. Gateway API Support

The Operator supports [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) as a modern alternative to Ingress for external access. Gateway API provides more expressive routing, better multi-tenancy support, and is more portable across implementations.

### 6.1 Prerequisites

- A Gateway API implementation installed in your cluster (e.g., Envoy Gateway, Istio, Cilium, Traefik)
- Gateway API CRDs installed
- An existing `Gateway` resource configured

### 6.2 Basic Gateway API Example

First, ensure you have a Gateway resource (typically managed by your platform team):

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: main-gateway
  namespace: gateway-system
spec:
  gatewayClassName: envoy
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - name: wildcard-cert
      allowedRoutes:
        namespaces:
          from: All
```

Then configure your OpenBaoCluster to use it:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: gateway-cluster
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
  gateway:
    enabled: true
    gatewayRef:
      name: main-gateway
      namespace: gateway-system
    hostname: bao.example.com
    path: /
  deletionPolicy: Retain
```

A full sample using Gateway API (including TLS and a realistic Gateway reference) is provided in `config/samples/openbao_v1alpha1_openbaocluster_gateway.yaml`.

### 6.3 Gateway Configuration Options

| Field | Description | Required |
|-------|-------------|----------|
| `enabled` | Enable Gateway API support | Yes |
| `gatewayRef.name` | Name of the Gateway resource | Yes |
| `gatewayRef.namespace` | Namespace of the Gateway (defaults to cluster namespace) | No |
| `hostname` | Hostname for routing (added to TLS SANs) | Yes |
| `path` | Path prefix for routing (defaults to `/`) | No |
| `annotations` | Annotations for the HTTPRoute | No |

### 6.4 What the Operator Creates

When Gateway API is enabled, the Operator creates an `HTTPRoute`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: gateway-cluster-httproute
  namespace: security
spec:
  parentRefs:
    - name: main-gateway
      namespace: gateway-system
  hostnames:
    - bao.example.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: gateway-cluster-public
          port: 8200
```

Verify the HTTPRoute was created:

```sh
kubectl -n security get httproutes
```

### 6.5 Gateway API vs Ingress

| Feature | Ingress | Gateway API |
|---------|---------|-------------|
| Configuration | `spec.ingress` | `spec.gateway` |
| Resource Created | `Ingress` | `HTTPRoute` |
| TLS Handling | Per-Ingress | Per-Gateway (shared) |
| Multi-tenancy | Limited | First-class support |
| Portability | Controller-specific annotations | Standardized API |

You can use either Ingress or Gateway API, but not both simultaneously on the same cluster.

### 6.6 End-to-End TLS with Gateway API

For end-to-end TLS (Gateway to OpenBao), configure a `BackendTLSPolicy` to trust the OpenBao CA. The operator automatically creates:

- A CA Secret named `<cluster-name>-tls-ca` containing the CA certificate.
- A ConfigMap with the same name containing the CA certificate in the `ca.crt` key (useful for implementations like Traefik that only support `ConfigMap` references).

Example `BackendTLSPolicy`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: openbaocluster-gateway-backend-tls
  namespace: openbao-operator-system
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: openbaocluster-gateway-public
  validation:
    caCertificateRefs:
      - group: ""
        kind: ConfigMap
        name: openbaocluster-gateway-tls-ca
    hostname: openbaocluster-gateway-public.openbao-operator-system.svc
```

This configuration tells the Gateway implementation to:

- Use HTTPS when talking to the OpenBao backend Service.
- Validate the backend certificate using the CA published by the operator.

Refer to `test/cluster/backend-tls-policy.yaml` for a complete, ready-to-apply example that matches the test cluster manifests.

Note: `BackendTLSPolicy` availability (including the exact `apiVersion`) depends on your Gateway API implementation and version.

## 7. Advanced Configuration

### 7.1 Audit Devices

Configure declarative audit devices for OpenBao clusters. Audit devices are created automatically on server startup and SIGHUP events.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: audit-cluster
spec:
  # ... other spec fields ...
  audit:
    - type: file
      path: stdout
      description: "Stdout audit device for debugging"
      options:
        file_path: "/dev/stdout"
        log_raw: "true"
    - type: file
      path: secure-audit
      description: "Secure audit logging"
      options:
        file_path: "/var/log/openbao/audit.log"
        format: "json"
```

See the [OpenBao audit documentation](https://openbao.org/docs/configuration/audit/) for available audit device types and options.

### 7.2 Plugins

Configure declarative plugins for OCI-based plugin management. Plugins are automatically downloaded and registered on server startup.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: plugin-cluster
spec:
  # ... other spec fields ...
  plugins:
    - type: secret
      name: aws
      image: "ghcr.io/openbao/openbao-plugin-secrets-aws"
      version: "v1.0.0"
      binaryName: "openbao-plugin-secrets-aws"
      sha256sum: "9fdd8be7947e4a4caf7cce4f0e02695081b6c85178aa912df5d37be97363144c"
    - type: auth
      name: kubernetes
      command: "openbao-plugin-auth-kubernetes"
      version: "v1.0.0"
      binaryName: "openbao-plugin-auth-kubernetes"
      sha256sum: "abc123..."
```

See the [OpenBao plugin documentation](https://openbao.org/docs/configuration/plugins/) for more details.

### 7.3 Telemetry

Configure telemetry reporting for metrics and observability.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: telemetry-cluster
spec:
  # ... other spec fields ...
  telemetry:
    prometheusRetentionTime: "24h"
    disableHostname: true
    metricsPrefix: "openbao_"
    # Prometheus-specific options
    prometheusRetentionTime: "24h"
    # Or StatsD options
    # statsdAddress: "statsd.example.com:8125"
    # Or DogStatsD options
    # dogStatsdAddress: "dogstatsd.example.com:8125"
    # dogStatsdTags:
    #   - "env:production"
    #   - "cluster:main"
```

See the [OpenBao telemetry documentation](https://openbao.org/docs/configuration/telemetry/) for all available options.

### 7.4 Custom Container Images

Override default container images for air-gapped environments or custom registries:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: custom-images-cluster
spec:
  # ... other spec fields ...
  backup:
    schedule: "0 3 * * *"
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "backups"
      pathPrefix: "clusters/custom-images-cluster"
      credentialsSecretRef:
        name: s3-credentials
    executorImage: "internal-registry.example.com/openbao/backup-executor:v0.1.0"
    # Preferred: Kubernetes Auth (automatically rotated tokens)
    kubernetesAuthRole: backup
    # Alternative: Static token (for self-init clusters without Kubernetes Auth)
    # tokenSecretRef:
    #   name: backup-token-secret
    retention:
      maxCount: 7
      maxAge: "168h"
  initContainer:
    enabled: true
    image: "internal-registry.example.com/openbao/openbao-config-init:v1.0.0"
```

**Note:** Using `:latest` tags for the init container is not recommended for production. Always specify a pinned version tag.

## 8. Backup Configuration

The operator supports scheduled backups of OpenBao Raft snapshots to S3-compatible object storage. Backups are executed using Kubernetes Jobs with a dedicated backup executor container.

### 8.1 Basic Backup Configuration

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  # ... other spec fields ...
  backup:
    schedule: "0 3 * * *"  # Daily at 3 AM
    executorImage: "openbao/backup-executor:v0.1.0"
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      pathPrefix: "clusters/backup-cluster"
      credentialsSecretRef:
        name: s3-credentials
        namespace: default
    retention:
      maxCount: 7
      maxAge: "168h"  # 7 days
```

### 8.2 Authentication Methods

The operator supports two authentication methods for backup operations:

#### 8.2.1 Kubernetes Auth (Preferred)

Kubernetes Auth uses ServiceAccount tokens that are automatically rotated by Kubernetes, providing better security than static tokens.

**Prerequisites:**
1. Enable Kubernetes authentication in OpenBao (via `spec.selfInit.requests`)
2. Create a Kubernetes Auth role that binds to the backup ServiceAccount
3. Configure `spec.backup.kubernetesAuthRole`

**Example Configuration:**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
  namespace: openbao
spec:
  selfInit:
    enabled: true
    requests:
      # Enable Kubernetes authentication
      - name: enable-kubernetes-auth
        operation: update
        path: sys/auth/kubernetes
        data:
          type: kubernetes
      # Configure Kubernetes auth
      - name: configure-kubernetes-auth
        operation: update
        path: auth/kubernetes/config
        data:
          kubernetes_host: "https://kubernetes.default.svc"
      # Create backup policy
      - name: create-backup-policy
        operation: update
        path: sys/policies/acl/backup
        data:
          policy: |
            path "sys/storage/raft/snapshot" {
              capabilities = ["read"]
            }
      # Create Kubernetes Auth role for backups
      # The ServiceAccount name is automatically <cluster-name>-backup-serviceaccount
      - name: create-backup-kubernetes-role
        operation: update
        path: auth/kubernetes/role/backup
        data:
          bound_service_account_names: backup-cluster-backup-serviceaccount
          bound_service_account_namespaces: openbao
          policies: backup
          ttl: 1h
  backup:
    schedule: "0 3 * * *"
    kubernetesAuthRole: backup  # Reference to the role created above
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
```

**Important:** The OpenBaoCluster's ServiceAccount must have the `system:auth-delegator` ClusterRole to verify ServiceAccount tokens. See `test/cluster/openbao-kubernetes-auth-rbac.yaml` for an example.

#### 8.2.2 Static Token (Fallback)

**Important:** Root tokens are no longer used for backup operations. All clusters (both standard and self-init) must use either Kubernetes Auth or a dedicated backup token Secret.

For clusters that cannot use Kubernetes Auth, you must create a backup token Secret with appropriate permissions and reference it via `spec.backup.tokenSecretRef`.

**Example for Self-Init Cluster:**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: backup-cluster
spec:
  selfInit:
    enabled: true
    requests:
      # Create backup policy and AppRole (see full example in samples)
      - name: create-backup-policy
        operation: update
        path: sys/policies/acl/backup
        data:
          policy: |
            path "sys/storage/raft/snapshot" {
              capabilities = ["read"]
            }
  backup:
    schedule: "0 3 * * *"
    tokenSecretRef:
      name: backup-token-secret
      namespace: openbao-operator-system
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
```

### 8.3 Backup ServiceAccount

The operator automatically creates `<cluster-name>-backup-serviceaccount` when backups are enabled. This ServiceAccount:
- Is used by backup Jobs for Kubernetes Auth
- Has its token automatically mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`
- Is owned by the OpenBaoCluster for automatic cleanup

### 8.4 Backup Execution

When a backup is scheduled:
1. The operator creates a Kubernetes Job with the backup executor container
2. The Job runs as the backup ServiceAccount
3. The executor:
   - Authenticates to OpenBao (Kubernetes Auth or static token)
   - Discovers the Raft leader
   - Streams the snapshot directly to object storage
   - Verifies the upload
4. Job status is monitored and backup status is updated in `Status.Backup`

### 8.5 Backup Status

Monitor backup status via the cluster status:

```sh
kubectl get openbaocluster backup-cluster -o yaml
```

Status fields:
- `Status.Backup.LastBackupTime`: Timestamp of last successful backup
- `Status.Backup.NextScheduledBackup`: Next scheduled backup time
- `Status.Backup.ConsecutiveFailures`: Number of consecutive failures
- `Status.Conditions`: `BackingUp` condition shows current backup state

### 8.6 Retention Policies

Configure automatic cleanup of old backups:

```yaml
backup:
  retention:
    maxCount: 7      # Keep only the 7 most recent backups
    maxAge: "168h"   # Delete backups older than 7 days
```

Retention is applied after successful backup upload. Deletion failures are logged but do not fail the backup.

### 8.7 Pre-Upgrade Snapshots

Enable automatic snapshots before upgrades:

```yaml
backup:
  preUpgradeSnapshot: true
```

This ensures you have a backup before any version upgrade begins.

## 9. Pausing Reconciliation

Set `spec.paused` to temporarily stop reconciliation for a cluster:

```sh
kubectl -n security patch openbaocluster dev-cluster \
  --type merge -p '{"spec":{"paused":true}}'
```

While paused:

- The Operator does not mutate StatefulSets, Secrets, or ConfigMaps for the cluster.
- Finalizers and delete handling still run if the CR is deleted.

Set `spec.paused` back to `false` to resume reconciliation.

## 10. Deletion and DeletionPolicy

### 8.1 Resource Ownership

All resources created by the operator (StatefulSet, Services, ConfigMaps, Secrets, ServiceAccounts, Ingresses, HTTPRoutes) have Kubernetes `OwnerReferences` pointing to the parent `OpenBaoCluster`. This ensures:

- **Automatic Garbage Collection:** When an `OpenBaoCluster` is deleted, Kubernetes automatically cascades the deletion to all owned resources.
- **Reconciliation on Drift:** The controller watches all owned resources, so any out-of-band modifications trigger immediate reconciliation to restore the desired state.
- **Clear Ownership:** Each resource is clearly associated with its parent cluster via the `OwnerReference`, making it easy to identify which resources belong to which cluster.

### 8.2 Deletion Policies

`spec.deletionPolicy` controls additional cleanup behavior when the `OpenBaoCluster` is deleted:

- `Retain` (default):
  - Kubernetes automatically deletes StatefulSet, Services, ConfigMap, and Secrets (via OwnerReferences).
  - Leaves PVCs and external backups intact.
- `DeletePVCs`:
  - Deletes PVCs labeled `openbao.org/cluster=<name>` in addition to the automatic cleanup.
  - Retains external backups.
- `DeleteAll` (future behavior):
  - Intended to also delete backup objects. Backup deletion will be wired into the BackupManager
    as that implementation is completed.

Delete the cluster:

```sh
kubectl -n security delete openbaocluster dev-cluster
```

Verify cleanup according to the chosen `deletionPolicy`:

```sh
kubectl -n security get statefulsets,svc,cm,secrets,pvc -l openbao.org/cluster=dev-cluster
```

## 11. Security Considerations

### 11.1 Root Token Secret

During cluster initialization (when **not** using self-initialization), the Operator stores the initial root token in a Secret named `<cluster>-root-token`. This token grants **full administrative access** to the OpenBao cluster.

**Note:** When `spec.selfInit.enabled = true`, no root token Secret is created. The root token is automatically revoked by OpenBao after self-initialization completes. See section 5 for details.

Retrieve the root token (for initial setup only, standard init only):

```sh
kubectl -n security get secret dev-cluster-root-token -o jsonpath='{.data.token}' | base64 -d
```

**Security Best Practices:**

- **Limit Access:** Configure RBAC to restrict access to the root token Secret. Only cluster administrators should have read access.
- **Rotate or Revoke:** After initial cluster setup, consider revoking the root token and creating more granular policies for ongoing administration.
- **Audit Access:** Enable Kubernetes audit logging to monitor access to this Secret.
- **Consider Self-Init:** For production workloads with GitOps workflows, consider using self-initialization to avoid having a root token Secret at all.
- **Not Auto-Deleted:** The root token Secret is NOT automatically deleted when the cluster is deleted (to support recovery scenarios). Clean it up manually if needed:
  ```sh
  kubectl -n security delete secret dev-cluster-root-token
  ```

### 11.2 Auto-Unseal Configuration

#### 11.2.1 Static Auto-Unseal (Default)

The Operator manages a static auto-unseal key stored in `<cluster>-unseal-key`. This key is used by OpenBao to automatically unseal on startup.

**Important:** If this Secret is compromised, an attacker with access to the Raft data can decrypt it. Ensure:
- etcd encryption at rest is enabled in your Kubernetes cluster.
- RBAC strictly limits access to Secrets in the OpenBao namespace.

**OpenBao Version Requirement:** The static auto-unseal feature requires **OpenBao v2.4.0 or later**. Earlier versions do not support the `seal "static"` configuration and will fail to start.

#### 11.2.2 External KMS Auto-Unseal

For enhanced security, you can configure external KMS providers (AWS KMS, GCP Cloud KMS, Azure Key Vault, or HashiCorp Vault Transit) to manage the unseal key instead of storing it in Kubernetes Secrets.

**Example: AWS KMS**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 3
  unseal:
    type: "awskms"
    options:
      kms_key_id: "arn:aws:kms:us-east-1:123456789012:key/abcd1234-5678-90ab-cdef-1234567890ab"
      region: "us-east-1"
    # Optional: If not using IRSA (IAM Roles for Service Accounts)
    credentialsSecretRef:
      name: aws-kms-credentials
      namespace: security
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

**Example: GCP Cloud KMS**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 3
  unseal:
    type: "gcpckms"
    options:
      project: "my-gcp-project"
      region: "us-central1"
      key_ring: "openbao-keyring"
      crypto_key: "openbao-unseal-key"
    # Optional: If not using GKE Workload Identity
    credentialsSecretRef:
      name: gcp-kms-credentials
      namespace: security
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

**Note:** When using external KMS, the operator does NOT create the `<cluster>-unseal-key` Secret. The Root of Trust shifts from Kubernetes Secrets to the cloud provider's KMS service. For GCP, the credentials Secret must contain a key named `credentials.json` with the GCP service account JSON credentials.

### 11.3 Image Verification (Supply Chain Security)

To protect against compromised registries and supply chain attacks, you can enable container image signature verification using Cosign. The operator verifies image signatures against the Rekor transparency log by default, providing strong non-repudiation guarantees.

**Example: Enable Image Verification**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  imageVerification:
    enabled: true
    publicKey: |
      -----BEGIN PUBLIC KEY-----
      MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
      -----END PUBLIC KEY-----
    failurePolicy: Block  # or "Warn" to log but proceed
    # ignoreTlog: false  # Set to true to disable Rekor transparency log verification
  replicas: 3
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

**Example: Image Verification with Private Registry**

When using images from private registries, provide ImagePullSecrets for authentication:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "private-registry.example.com/openbao/openbao:2.1.0"
  imageVerification:
    enabled: true
    publicKey: |
      -----BEGIN PUBLIC KEY-----
      MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
      -----END PUBLIC KEY-----
    failurePolicy: Block
    imagePullSecrets:
      - name: registry-credentials  # Secret of type kubernetes.io/dockerconfigjson
  replicas: 3
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

**Configuration Options:**
- `enabled`: Enable or disable image verification (required).
- `publicKey`: PEM-encoded Cosign public key used to verify signatures (required when enabled).
- `failurePolicy`: Behavior on verification failure:
  - `Block` (default): Sets `ConditionDegraded=True` with `Reason=ImageVerificationFailed` and blocks StatefulSet updates.
  - `Warn`: Logs an error and emits a Kubernetes Event but proceeds with deployment.
- `ignoreTlog`: Set to `true` to disable Rekor transparency log verification (default: `false`). When `false`, signatures are verified against Rekor for non-repudiation, following OpenBao's verification guidance.
- `imagePullSecrets`: List of ImagePullSecrets (type `kubernetes.io/dockerconfigjson` or `kubernetes.io/dockercfg`) for private registry authentication during verification.

**Security Features:**
- **TOCTOU Mitigation**: The operator resolves image tags to digests during verification and uses the verified digest in StatefulSets, preventing Time-of-Check to Time-of-Use attacks where a tag could be updated between verification and deployment.
- **Rekor Verification**: By default, signatures are verified against the Rekor transparency log, providing strong non-repudiation guarantees as recommended by OpenBao.
- **Private Registry Support**: ImagePullSecrets can be provided for authentication when verifying images from private registries.

**Note:** Image verification results are cached in-memory keyed by image digest (not tag) and public key to avoid redundant network calls while preventing cache issues when tags change.

## 12. Multi-Tenancy Security

When running multiple `OpenBaoCluster` resources in a shared Kubernetes cluster (multi-tenant), additional security considerations apply.

### 12.1 RBAC for Tenant Isolation

The operator provides ClusterRoles that can be bound at the namespace level to isolate tenant access:

```sh
# See example RBAC configurations
kubectl apply -f config/samples/namespace_scoped_openbaocluster-rbac.yaml
```

**Key points:**

- Use `RoleBinding` (not `ClusterRoleBinding`) to restrict users to their namespace
- The `openbaocluster-editor-role` allows creating/managing clusters but NOT reading Secrets
- Platform teams should have `openbaocluster-admin-role` cluster-wide

### 12.2 Secret Access Isolation

The operator creates sensitive Secrets for each cluster:
- `<cluster>-root-token` - Full admin access to OpenBao
- `<cluster>-unseal-key` - Can decrypt all OpenBao data
- `<cluster>-tls-ca` / `<cluster>-tls-server` - TLS credentials

**Critical:** Tenants should NOT have direct access to these Secrets. Configure RBAC to restrict Secret access:

```yaml
# Example: Deny Secret access in tenant namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: deny-openbao-secrets
  namespace: team-a-prod
rules:
  # Explicitly grant access only to non-sensitive secrets
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
    resourceNames: []  # Empty = no access to any secrets by name
---
# For production, consider using OPA/Gatekeeper or Kyverno to enforce
# that tenants cannot read secrets matching *-root-token, *-unseal-key patterns
```

**Recommendation:** Use a policy engine (OPA Gatekeeper, Kyverno) to enforce Secret access restrictions:

```yaml
# Example Kyverno policy to block access to sensitive OpenBao secrets
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: block-openbao-sensitive-secrets
spec:
  validationFailureAction: Enforce
  background: false
  rules:
    - name: block-root-token-access
      match:
        any:
          - resources:
              kinds:
                - Secret
              names:
                - "*-root-token"
                - "*-unseal-key"
      exclude:
        any:
          - subjects:
              - kind: ServiceAccount
                name: openbao-operator-controller-manager
                namespace: openbao-operator-system
      validate:
        message: "Access to OpenBao root token and unseal key secrets is restricted"
        deny: {}
```

### 12.3 Network Policies

The operator automatically creates a NetworkPolicy for each OpenBaoCluster that enforces default-deny ingress and restricts egress. **Backup job pods are excluded from this NetworkPolicy** (they have the label `openbao.org/component=backup`) to allow access to object storage services.

If you need to restrict backup job network access, create a custom NetworkPolicy targeting backup jobs:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: backup-job-network-policy
  namespace: team-a-prod
spec:
  podSelector:
    matchLabels:
      openbao.org/component: backup
      openbao.org/cluster: team-a-cluster
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow ingress from operator (if needed for monitoring)
    - from:
        - namespaceSelector:
            matchLabels:
              app.kubernetes.io/name: openbao-operator
      ports:
        - port: 8080  # metrics port
  egress:
    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    # Allow access to object storage (adjust namespace/port as needed)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: rustfs
      ports:
        - port: 9000
          protocol: TCP
    # Allow access to OpenBao cluster for snapshot API
    - to:
        - podSelector:
            matchLabels:
              openbao.org/cluster: team-a-cluster
      ports:
        - port: 8200
          protocol: TCP
```

For general cluster isolation, the operator-managed NetworkPolicy provides:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openbao-cluster-isolation
  namespace: team-a-prod
spec:
  podSelector:
    matchLabels:
      openbao.org/cluster: team-a-cluster
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow traffic only from pods in the same cluster
    - from:
        - podSelector:
            matchLabels:
              openbao.org/cluster: team-a-cluster
      ports:
        - port: 8200
        - port: 8201
    # Allow traffic from ingress controller (if needed)
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
      ports:
        - port: 8200
  egress:
    # Allow cluster-internal communication
    - to:
        - podSelector:
            matchLabels:
              openbao.org/cluster: team-a-cluster
      ports:
        - port: 8200
        - port: 8201
    # Allow DNS
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - port: 53
          protocol: UDP
    # Allow Kubernetes API (for auto-join discovery via discover-k8s provider)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: default
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 443
          protocol: TCP
```

### 12.4 Resource Quotas

Prevent one tenant from exhausting cluster resources:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: openbao-quota
  namespace: team-a-prod
spec:
  hard:
    # Limit number of OpenBao pods
    pods: "10"
    # Limit total PVC storage
    requests.storage: "100Gi"
    persistentvolumeclaims: "5"
    # Limit CPU/Memory
    requests.cpu: "4"
    requests.memory: "8Gi"
    limits.cpu: "8"
    limits.memory: "16Gi"
```

### 12.5 Backup Credential Isolation

Each tenant should have **separate backup credentials** with access only to their backup prefix:

```yaml
# Tenant A backup credentials - access only to tenant-a/ prefix
apiVersion: v1
kind: Secret
metadata:
  name: backup-credentials-team-a
  namespace: team-a-prod
type: Opaque
stringData:
  accessKeyId: "TENANT_A_ACCESS_KEY"
  secretAccessKey: "TENANT_A_SECRET_KEY"
  # Region is optional
```

Configure the S3/object storage IAM policy to restrict access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::openbao-backups/team-a-prod/*",
        "arn:aws:s3:::openbao-backups"
      ],
      "Condition": {
        "StringLike": {
          "s3:prefix": ["team-a-prod/*"]
        }
      }
    }
  ]
}
```

Then reference in the OpenBaoCluster:

```yaml
spec:
  backup:
    schedule: "0 3 * * *"
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      pathPrefix: "team-a-prod/"  # Isolated prefix per tenant
      credentialsSecretRef:
        name: backup-credentials-team-a
        namespace: team-a-prod
```

### 12.6 Pod Security Standards

OpenBao pods created by the operator run with these security settings:
- Non-root user (UID 100, GID 1000)
- Read-only root filesystem (secrets/config mounted read-only)
- No privilege escalation

Ensure your cluster's Pod Security Admission is configured to allow these pods:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: team-a-prod
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

### 12.7 Operator Audit Logging

The Operator emits structured audit logs for critical operations, tagged with `audit=true` and `event_type` for easy filtering in log aggregation systems. Audit events are logged for:

- **Cluster Initialization:** `Init`, `InitCompleted`, `InitFailed`
- **Leader Step-Down:** `StepDown`, `StepDownCompleted`, `StepDownFailed` (during upgrades)

These audit logs are distinct from regular debug/info logs and can be filtered using the `audit=true` label.

### 12.8 Kubernetes Audit Logging

Enable Kubernetes audit logging to monitor access to sensitive resources:

```yaml
# Example audit policy for OpenBao-related resources
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Log all access to OpenBao secrets
  - level: RequestResponse
    resources:
      - group: ""
        resources: ["secrets"]
    namespaces: ["*"]
    verbs: ["get", "list", "watch"]
    omitStages:
      - RequestReceived
  # Log all OpenBaoCluster changes
  - level: RequestResponse
    resources:
      - group: "openbao.org"
        resources: ["openbaoclusters"]
    verbs: ["create", "update", "patch", "delete"]
```

### 12.9 Etcd Encryption Warning

The Operator sets an `EtcdEncryptionWarning` condition to remind users that security relies on underlying Kubernetes secret encryption at rest. The operator cannot verify etcd encryption status, so this condition is always set to `True` with a message reminding users to ensure etcd encryption is enabled.

Check the condition:

```sh
kubectl -n security get openbaocluster dev-cluster -o jsonpath='{.status.conditions[?(@.type=="EtcdEncryptionWarning")]}'
```

### 12.10 Multi-Tenancy Checklist

Before deploying OpenBao in a multi-tenant environment, verify:

- [ ] Namespace-scoped RoleBindings configured for each tenant
- [ ] Tenants cannot read `*-root-token` and `*-unseal-key` Secrets
- [ ] NetworkPolicies isolate OpenBao clusters from each other (operator-managed NetworkPolicy excludes backup jobs; create custom NetworkPolicy for backup jobs if restrictions are needed)
- [ ] ResourceQuotas limit tenant resource consumption
- [ ] Each tenant has separate backup credentials with isolated bucket prefixes
- [ ] Pod Security Admission configured appropriately
- [ ] Kubernetes audit logging enabled for Secret access
- [ ] etcd encryption at rest enabled for Secrets (operator will show `EtcdEncryptionWarning` condition if status cannot be verified)
- [ ] Consider using self-initialization to avoid root token Secrets entirely
- [ ] Operator-managed NetworkPolicy is in place (automatically created by the Operator)
- [ ] Container images are pinned to specific versions (not using `:latest` tags)
- [ ] Operator audit logs are being collected and monitored (filter by `audit=true` label)

## 13. Next Steps

Once you have basic cluster creation working:

- Experiment with `spec.config` to inject additional, non-protected OpenBao configuration (validated via allowlist).
- Configure audit devices via `spec.audit` for declarative audit logging.
- Configure plugins via `spec.plugins` for OCI-based plugin management.
- Configure telemetry via `spec.telemetry` for metrics and observability.
- Configure `spec.backup` and object storage to enable snapshot streaming (see the TDD for details).
- Integrate Operator metrics with your monitoring stack using the manifests under `config/prometheus`.

For deeper architectural details and the full roadmap (upgrades, backups, multi-tenancy),
refer to `docs/technical-design-document.md` and `docs/implementation-plan.md`.
