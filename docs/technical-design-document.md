# Technical Design Document: OpenBao Supervisor Operator



## 1\. Executive Summary

The OpenBao Operator acts as an autonomous administrator for OpenBao clusters on Kubernetes. Unlike legacy operators that attempt to micro-manage internal consensus (Raft), this Operator adopts a **Supervisor Pattern**. It delegates data consistency to the OpenBao binary (via `retry_join` and static auto-unseal) while managing the external ecosystem: PKI lifecycle, Infrastructure state, and Safe Version Upgrades.

The design is intentionally **cloud-agnostic** and **multi-tenant**:

- Cloud-agnostic: the Operator relies only on standard Kubernetes APIs and generic HTTP/S-compatible object storage, without hard-coding provider-specific features.
- Multi-tenant: a single Operator instance manages multiple `OpenBaoCluster` resources across namespaces, while ensuring isolation and independence between clusters.

## 2\. System Architecture

The Operator is composed of a single Controller Manager running multiple concurrent reconciliation loops (Sub-Controllers).

### 2.1 Component Interaction

1.  **User:** Submits `OpenBaoCluster` Custom Resource (CR).
2.  **CertController:** Observes CR, generates CA and Leaf Certs into Kubernetes Secrets.
3.  **ConfigController:** Generates the `config.hcl` (ConfigMap) injecting TLS paths and `retry_join` stanzas.
4.  **StatefulSetController:** Ensures the StatefulSet exists, matches the desired version, and mounts the Secrets/ConfigMaps.
5.  **UpgradeController:** Intercepts version changes to perform Raft-aware rolling updates.
6.  **OpenBao Pods:** Boot up, mount certs, read config, and auto-join the cluster using the K8s API peer discovery.

### 2.2 Assumptions and Non-Goals

**Assumptions**

- The Kubernetes cluster provides:
  - A default StorageClass for persistent volumes.
  - Working DNS for StatefulSet pod and service names.
  - A mechanism to provide credentials for object storage (e.g., Secrets, workload identity).
- **OpenBao Version:** The Operator uses static auto-unseal, which requires **OpenBao v2.4.0 or later**. Versions below 2.4.0 do not support the `seal "static"` configuration and will fail to start.
- Platform teams are responsible for:
  - Deploying and upgrading the Operator itself (or enabling an optional self-managed mode).
  - Wiring the Operator and OpenBao telemetry into their observability stack.

**Non-Goals (v0.1)**

- Automated restore workflows from backups (restores may be manual).
- Cross-cluster or multi-region OpenBao federation.
- Provider-specific integrations (e.g., native AWS/GCP/Azure APIs); these can be added as optional extensions.

-----

## 3\. API Specification (Custom Resource Definition)

We define the API using Go structs which control the CRD generation.

### 3.1 Spec (Desired State)

```go
type OpenBaoClusterSpec struct {
    // General
    // Semantic OpenBao version, used for upgrade orchestration.
    Version  string `json:"version"`  // e.g., "2.1.0"
    // Container image to run; defaults may be derived from Version.
    Image    string `json:"image"`    // e.g., "openbao/openbao:2.1.0"
    Replicas int32  `json:"replicas"` // Default: 3

    // If true, all reconciliation for this OpenBaoCluster is paused (maintenance mode).
    Paused   bool   `json:"paused,omitempty"`

    // Security (The "Killer Feature")
    TLS TLSConfig `json:"tls"`

    // Infrastructure
    Storage  StorageConfig     `json:"storage"`

    // Networking / Exposure
    Service *ServiceConfig `json:"service,omitempty"`
    Ingress *IngressConfig `json:"ingress,omitempty"`

    // Optional user-supplied OpenBao configuration fragments.
    // The Operator uses an allowlist-based validation approach, only allowing
    // valid OpenBao configuration parameters. Operator-managed stanzas
    // (e.g., listener "tcp", storage "raft", seal "static") are excluded
    // from the allowlist and cannot be overridden.
    Config   map[string]string `json:"config,omitempty"`

    // Audit configures declarative audit devices for the OpenBao cluster.
    // See: https://openbao.org/docs/configuration/audit/
    Audit    []AuditDevice     `json:"audit,omitempty"`

    // Plugins configures declarative plugins for the OpenBao cluster.
    // See: https://openbao.org/docs/configuration/plugins/
    Plugins  []Plugin          `json:"plugins,omitempty"`

    // Telemetry configures telemetry reporting for the OpenBao cluster.
    // See: https://openbao.org/docs/configuration/telemetry/
    Telemetry *TelemetryConfig `json:"telemetry,omitempty"`

    // Operations
    Backup   *BackupSchedule   `json:"backup,omitempty"`

    // Deletion behavior when the OpenBaoCluster is removed.
    // e.g., "Retain" (keep PVCs and backups), "DeletePVCs", "DeleteAll".
    DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

    // SelfInit configures OpenBao's native self-initialization feature.
    // When enabled, OpenBao initializes itself on first start using the
    // configured requests, and the root token is automatically revoked.
    // See: https://openbao.org/docs/configuration/self-init/
    SelfInit *SelfInitConfig `json:"selfInit,omitempty"`

    // Gateway configures Kubernetes Gateway API access (alternative to Ingress).
    Gateway *GatewayConfig `json:"gateway,omitempty"`

    // InitContainer configures the init container used to render OpenBao configuration.
    // The init container renders the final config.hcl from a template using environment
    // variables such as HOSTNAME and POD_IP.
    InitContainer *InitContainerConfig `json:"initContainer,omitempty"`
}

type TLSConfig struct {
    Enabled        bool   `json:"enabled"`
    // Duration string for rotation, e.g., "720h"
    RotationPeriod string `json:"rotationPeriod"` 
    // SANs to inject into certs for external access
    ExtraSANs      []string `json:"extraSANs,omitempty"` 
}

// StorageConfig captures the storage-related configuration for the StatefulSet.
type StorageConfig struct {
    // Size of the persistent volume (e.g., "10Gi").
    Size string `json:"size"`
    // Optional storageClassName for the PVCs.
    StorageClassName *string `json:"storageClassName,omitempty"`
}

// ServiceConfig controls how the main OpenBao Service is exposed inside/outside the cluster.
type ServiceConfig struct {
    // Kubernetes ServiceType, e.g., "ClusterIP", "LoadBalancer".
    Type corev1.ServiceType `json:"type,omitempty"`
    // Additional annotations to apply to the Service.
    Annotations map[string]string `json:"annotations,omitempty"`
}

// IngressConfig controls optional HTTP(S) ingress in front of the OpenBao Service.
type IngressConfig struct {
    // If true, the Operator creates and manages an Ingress for external access.
    Enabled bool `json:"enabled"`
    // Optional IngressClassName (e.g., "nginx", "traefik", "alb").
    ClassName *string `json:"className,omitempty"`
    // Primary host for external access (e.g., "bao.example.com").
    Host string `json:"host"`
    // HTTP path to route to OpenBao (defaults to "/").
    Path string `json:"path,omitempty"`
    // Optional TLS Secret name; if empty, the Operator uses the cluster TLS Secret.
    TLSSecretName string `json:"tlsSecretName,omitempty"`
    // Additional annotations to apply to the Ingress.
    Annotations map[string]string `json:"annotations,omitempty"`
}

// SelfInitConfig enables OpenBao's self-initialization feature.
// See: https://openbao.org/docs/configuration/self-init/
type SelfInitConfig struct {
    // Enabled activates OpenBao's self-initialization feature.
    // When true, the Operator injects `initialize` stanzas into config.hcl
    // and does NOT create a root token Secret (root token is auto-revoked).
    Enabled bool `json:"enabled"`
    
    // Requests defines the API operations to execute during self-initialization.
    // Each request becomes a named `request` block inside an `initialize` stanza.
    Requests []SelfInitRequest `json:"requests,omitempty"`
}

// SelfInitRequest defines a single API operation to execute during self-initialization.
type SelfInitRequest struct {
    // Name is a unique identifier for this request (used as the block name).
    // Must match regex ^[A-Za-z_][A-Za-z0-9_-]*$
    Name string `json:"name"`
    
    // Operation is the API operation type: "create", "read", "update", "delete", "list".
    Operation string `json:"operation"`
    
    // Path is the API path to call (e.g., "sys/audit/stdout", "auth/kubernetes/config").
    Path string `json:"path"`
    
    // Data contains the request payload as a structured map.
    // This must be a JSON/YAML object whose shape matches the target API
    // endpoint. Nested maps and lists are supported and are rendered into
    // the initialize stanza as HCL objects (for example, the "options" map
    // used by audit devices).
    //
    // This payload is stored in the OpenBaoCluster resource and persisted
    // in etcd; it must not contain sensitive values such as tokens,
    // passwords, or unseal keys.
    Data map[string]interface{} `json:"data,omitempty"`
    
    // AllowFailure allows this request to fail without blocking initialization.
    // Defaults to false.
    AllowFailure bool `json:"allowFailure,omitempty"`
}

// GatewayConfig configures Kubernetes Gateway API access for the OpenBao cluster.
// This is an alternative to Ingress for external access, using the more modern
// and expressive Gateway API.
type GatewayConfig struct {
    // Enabled activates Gateway API support for this cluster.
    // When true, the Operator creates an HTTPRoute for the cluster.
    Enabled bool `json:"enabled"`
    
    // GatewayRef references an existing Gateway resource that will handle
    // traffic for this OpenBao cluster. The Gateway must already exist.
    GatewayRef GatewayReference `json:"gatewayRef"`
    
    // Hostname for routing traffic to this OpenBao cluster.
    // This hostname will be automatically added to the TLS SANs.
    Hostname string `json:"hostname"`
    
    // Path prefix for the HTTPRoute (defaults to "/").
    Path string `json:"path,omitempty"`
    
    // Annotations to apply to the HTTPRoute resource.
    Annotations map[string]string `json:"annotations,omitempty"`
}

// GatewayReference identifies a Gateway resource.
type GatewayReference struct {
    // Name of the Gateway resource.
    Name string `json:"name"`
    
    // Namespace of the Gateway resource. If empty, uses the OpenBaoCluster namespace.
    Namespace string `json:"namespace,omitempty"`
}

// BackupSchedule defines when and where snapshots are stored.
type BackupSchedule struct {
    // Cron-style schedule, e.g., "0 3 * * *".
    Schedule string `json:"schedule"`
    // Target object storage configuration.
    Target BackupTarget `json:"target"`
    // Retention defines optional backup retention policy.
    Retention *BackupRetention `json:"retention,omitempty"`
    // PreUpgradeSnapshot, when true, triggers a backup before any upgrade.
    PreUpgradeSnapshot bool `json:"preUpgradeSnapshot,omitempty"`
    // ExecutorImage is the container image to use for backup operations.
    // Defaults to "openbao/backup-executor:v0.1.0" if not specified.
    // This allows users to override the image for air-gapped environments or custom registries.
    ExecutorImage string `json:"executorImage,omitempty"`
}

// BackupRetention defines retention policy for backups.
type BackupRetention struct {
    // MaxCount is the maximum number of backups to retain (0 = unlimited).
    MaxCount int32 `json:"maxCount,omitempty"`
    // MaxAge is the maximum age of backups to retain, e.g., "168h" for 7 days.
    // Backups older than this are deleted after successful new backup upload.
    MaxAge string `json:"maxAge,omitempty"`
}

// BackupTarget describes a generic, cloud-agnostic object storage destination.
type BackupTarget struct {
    // HTTP(S) endpoint for the object storage service.
    Endpoint string `json:"endpoint"`
    // Bucket/container name.
    Bucket string `json:"bucket"`
    // Optional prefix within the bucket for this cluster's snapshots.
    PathPrefix string `json:"pathPrefix,omitempty"`
    // Optional reference to a Secret containing credentials or configuration
    // required to authenticate to the object storage provider.
    CredentialsSecretRef *corev1.SecretReference `json:"credentialsSecretRef,omitempty"`
}

// AuditDevice defines a declarative audit device configuration.
// See: https://openbao.org/docs/configuration/audit/
type AuditDevice struct {
    Type        string                `json:"type"`        // e.g., "file", "syslog"
    Path        string                `json:"path"`        // path in root namespace
    Description string                `json:"description,omitempty"`
    Options     *apiextensionsv1.JSON `json:"options,omitempty"` // device-specific options
}

// Plugin defines a declarative plugin configuration.
// See: https://openbao.org/docs/configuration/plugins/
type Plugin struct {
    Type       string   `json:"type"`       // e.g., "secret", "auth"
    Name       string   `json:"name"`
    Image      string   `json:"image,omitempty"`      // OCI image URL (or Command)
    Command    string   `json:"command,omitempty"`   // manually downloaded plugin
    Version    string   `json:"version"`
    BinaryName string   `json:"binaryName"`
    SHA256Sum  string   `json:"sha256sum"`   // 64-char hex
    Args       []string `json:"args,omitempty"`
    Env        []string `json:"env,omitempty"`
}

// TelemetryConfig defines telemetry reporting configuration.
// See: https://openbao.org/docs/configuration/telemetry/
type TelemetryConfig struct {
    // Common options
    UsageGaugePeriod          string   `json:"usageGaugePeriod,omitempty"`
    MaximumGaugeCardinality   *int32   `json:"maximumGaugeCardinality,omitempty"`
    DisableHostname           bool     `json:"disableHostname,omitempty"`
    EnableHostnameLabel       bool     `json:"enableHostnameLabel,omitempty"`
    MetricsPrefix             string   `json:"metricsPrefix,omitempty"`
    LeaseMetricsEpsilon       string   `json:"leaseMetricsEpsilon,omitempty"`
    // Provider-specific options (Prometheus, Statsite, StatsD, DogStatsD, Circonus, Stackdriver)
    // See OpenBao telemetry documentation for full list
}

// DeletionPolicy defines what happens to underlying resources when the CR is deleted.
type DeletionPolicy string

const (
    DeletionPolicyRetain     DeletionPolicy = "Retain"     // Keep PVCs and external backups
    DeletionPolicyDeletePVCs DeletionPolicy = "DeletePVCs" // Delete StatefulSet and PVCs, keep external backups
    DeletionPolicyDeleteAll  DeletionPolicy = "DeleteAll"  // Delete StatefulSet, PVCs, and attempt to delete external backups
)
```

### 3.2 Status (Observability)

We adhere to Kubernetes API guidelines using `Conditions`.

```go
// ClusterPhase is a high-level summary of cluster state.
type ClusterPhase string

const (
    ClusterPhaseInitializing ClusterPhase = "Initializing"
    ClusterPhaseRunning      ClusterPhase = "Running"
    ClusterPhaseUpgrading    ClusterPhase = "Upgrading"
    ClusterPhaseBackingUp    ClusterPhase = "BackingUp"
    ClusterPhaseFailed       ClusterPhase = "Failed"
)

type OpenBaoClusterStatus struct {
    // High-level summary for the user.
    Phase ClusterPhase `json:"phase"`

    // The current Raft Leader pod name (discovered via API), e.g., "prod-cluster-0".
    ActiveLeader string `json:"activeLeader,omitempty"`

    // Critical for ensuring the StatefulSet is stable.
    ReadyReplicas int32 `json:"readyReplicas"`

    // Tracks the current OpenBao version running on the cluster (vs Spec.Version).
    CurrentVersion string `json:"currentVersion"`

    // Indicates whether the OpenBao cluster has been initialized via bao operator init
    // or self-initialization. Set to true after initialization completes.
    // Used by InfraManager to determine when to scale up from 1 replica.
    Initialized bool `json:"initialized,omitempty"`

    // SelfInitialized indicates whether the cluster was initialized using
    // OpenBao's self-initialization feature. When true, no root token Secret
    // exists for this cluster (the root token was auto-revoked).
    SelfInitialized bool `json:"selfInitialized,omitempty"`

    // Last successful backup timestamp (if backups are configured).
    // Deprecated: Use Backup.LastBackupTime instead.
    LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

    // Upgrade tracks the state of an in-progress upgrade (if any).
    Upgrade *UpgradeProgress `json:"upgrade,omitempty"`

    // Backup tracks the state of backups for this cluster.
    Backup *BackupStatus `json:"backup,omitempty"`

    // Conditions: Available, Upgrading, Degraded, BackingUp, etc.
    Conditions []metav1.Condition `json:"conditions"`
}

// UpgradeProgress tracks the state of an in-progress upgrade.
type UpgradeProgress struct {
    // TargetVersion is the version being upgraded to.
    TargetVersion string `json:"targetVersion"`
    // FromVersion is the version being upgraded from.
    FromVersion string `json:"fromVersion"`
    // StartedAt is when the upgrade began.
    StartedAt *metav1.Time `json:"startedAt,omitempty"`
    // CurrentPartition is the current StatefulSet partition value.
    CurrentPartition int32 `json:"currentPartition"`
    // CompletedPods lists ordinals of pods that have been successfully upgraded.
    CompletedPods []int32 `json:"completedPods,omitempty"`
    // LastStepDownTime records when the last leader step-down was performed.
    LastStepDownTime *metav1.Time `json:"lastStepDownTime,omitempty"`
}

// BackupStatus tracks the state of backups for a cluster.
type BackupStatus struct {
    // LastBackupTime is the timestamp of the last successful backup.
    LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
    // LastBackupSize is the size in bytes of the last successful backup.
    LastBackupSize int64 `json:"lastBackupSize,omitempty"`
    // LastBackupDuration is how long the last backup took (e.g., "45s").
    LastBackupDuration string `json:"lastBackupDuration,omitempty"`
    // LastBackupName is the object key/path of the last successful backup.
    LastBackupName string `json:"lastBackupName,omitempty"`
    // NextScheduledBackup is when the next backup is scheduled.
    NextScheduledBackup *metav1.Time `json:"nextScheduledBackup,omitempty"`
    // ConsecutiveFailures is the number of consecutive backup failures.
    ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
    // LastFailureReason describes why the last backup failed (if applicable).
    LastFailureReason string `json:"lastFailureReason,omitempty"`
}

// ConditionType identifies a specific aspect of cluster health or lifecycle.
type ConditionType string

const (
    // ConditionAvailable indicates whether the cluster is generally available.
    ConditionAvailable ConditionType = "Available"
    // ConditionTLSReady indicates whether TLS assets have been successfully provisioned.
    ConditionTLSReady ConditionType = "TLSReady"
    // ConditionUpgrading indicates whether an upgrade is currently in progress.
    ConditionUpgrading ConditionType = "Upgrading"
    // ConditionBackingUp indicates whether a backup is currently in progress.
    ConditionBackingUp ConditionType = "BackingUp"
    // ConditionDegraded indicates the operator has detected a problem requiring attention.
    ConditionDegraded ConditionType = "Degraded"
)

### 3.3 Example CRDs

**Minimal Cluster**

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
  paused: false
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: 10Gi
    # storageClassName: "gp2"
  deletionPolicy: Retain
```

**Cluster with Backups**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 5
  tls:
    enabled: true
    rotationPeriod: "720h"
    extraSANs:
      - "bao.example.com"
  backup:
    schedule: "0 3 * * *"
    target:
      endpoint: "https://object-storage.example.com"
      bucket: "openbao-backups"
      pathPrefix: "prod/"
  deletionPolicy: DeletePVCs
```

**Cluster with Self-Initialization**

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
    size: 10Gi
  selfInit:
    enabled: true
    requests:
      - name: enable-stdout-audit
        operation: update
        path: sys/audit/stdout
        data:
          type: file
          options:
            file_path: /dev/stdout
            log_raw: true
      - name: enable-kubernetes-auth
        operation: update
        path: sys/auth/kubernetes
        data:
          type: kubernetes
      - name: configure-kubernetes-auth
        operation: update
        path: auth/kubernetes/config
        data:
          kubernetes_host: "https://kubernetes.default.svc"
  deletionPolicy: Retain
```

**Cluster with Gateway API**

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
    size: 10Gi
  gateway:
    enabled: true
    gatewayRef:
      name: main-gateway
      namespace: gateway-system
    hostname: bao.example.com
    path: /
  deletionPolicy: Retain
```

-----

## 4\. Detailed Controller Design

The Operator runs a single `OpenBaoCluster` controller which delegates to four specific internal "Managers" to separate concerns.

All controllers MUST honor `spec.paused`:

- When `OpenBaoCluster.Spec.Paused == true`, reconcilers SHOULD short-circuit early and avoid mutating cluster resources, allowing safe manual maintenance (e.g., manual restore).
- Finalizers and deletion handling MAY still proceed to ensure cleanup when the CR is deleted.

### 4.1 The CertManager (TLS Lifecycle)

**Responsibility:** Bootstrap PKI and Rotate Certificates (or wait for external Secrets in External mode).

The CertManager supports two modes controlled by `spec.tls.mode`:

#### OperatorManaged Mode (Default)

When `spec.tls.mode` is `OperatorManaged` (or omitted), the operator generates and manages certificates:

  * **Logic Flow:**
    1.  **Reconcile:** Check if `Secret/<cluster>-tls-ca` exists.
    2.  **Bootstrap:** If missing, generate ECDSA P-256 Root CA. Store PEM in Secret.
    3.  **Issue:** Check if `Secret/<cluster>-tls-server` exists and is valid (for example, expiry \> configured rotation window).
    4.  **Rotate:** If the server certificate is within the rotation window, regenerate it using the CA and update the Secret atomically.
    5.  **Trigger:** Compute a SHA256 hash of the active server certificate and annotate OpenBao pods with it (for example, `openbao.org/tls-cert-hash`) so the operator and in-pod components can track which certificate each pod is using.
    6.  **Hot Reload:** When the hash changes, the operator uses a `ReloadSignaler` to update the annotation on each ready OpenBao pod. A sidecar container running in the same pod is responsible for watching this annotation or the mounted TLS volume and sending `SIGHUP` to the OpenBao process locally, avoiding the need for `pods/exec` privileges in the operator.

#### External Mode

When `spec.tls.mode` is `External`, the operator does not generate or rotate certificates:

  * **Logic Flow:**
    1.  **Wait:** Check if `Secret/<cluster>-tls-ca` exists. If missing, log "Waiting for external TLS CA Secret" and return (no error).
    2.  **Wait:** Check if `Secret/<cluster>-tls-server` exists. If missing, log "Waiting for external TLS server Secret" and return (no error).
    3.  **Monitor:** When both secrets exist, compute the SHA256 hash of the server certificate.
    4.  **Hot Reload:** Trigger hot-reload via `ReloadSignaler` when the certificate hash changes (enables seamless rotation by cert-manager or other external providers).
    5.  **No Rotation:** Do not check expiry or attempt to rotate certificates; assume the external provider handles this.

**Reconciliation Semantics**

- **Idempotency:** Re-running reconciliation with the same Spec MUST lead to the same Secrets and annotations.
- **Backoff:** Transient errors (e.g., failed Secret update due to concurrent modification) are retried with exponential backoff.
- **Conditions:** Failures update a `TLSReady=False` condition with a clear reason and message.
- **External Mode:** The operator waits gracefully for external Secrets without erroring, allowing cert-manager or other tools time to provision certificates.

### 4.2 The InfrastructureManager (Config & StatefulSet)

**Responsibility:** Render configuration and maintain the StatefulSet.

  * **Configuration Strategy:**
      * We do not use a static ConfigMap. We generate it dynamically based on the cluster topology and merge in user-supplied configuration where safe.
      * **Injection:** We explicitly inject the `listener "tcp"` block pointing to the locations where we mounted the `CertManager` secrets (`/etc/bao/tls`).
  * **Discovery Strategy:**
      * **Bootstrap:** During initial cluster creation, we configure a single `retry_join` with `leader_api_addr` pointing to the deterministic DNS name of pod-0 (for example, `cluster-0.cluster-name.namespace.svc`). This ensures a stable leader for Day 0 initialization.
      * **Post-Initialization:** After the cluster is marked initialized, we switch to a Kubernetes go-discover based `retry_join` that uses `auto_join = "provider=k8s namespace=<ns> label_selector=\"openbao.org/cluster=<cluster-name>\""` so new pods can join dynamically based on labels rather than a static peer list.

  * **Auto-Unseal Integration:**
      * **Static Seal (Default):** If `spec.unseal` is omitted or `spec.unseal.type` is `"static"`:
        * On first reconcile, checks for the existence of `Secret/<cluster>-unseal-key` (per-cluster name).
        * If missing, generates 32 cryptographically secure random bytes and stores them as the unseal key in the Secret.
        * Mounts this Secret into the StatefulSet PodSpec at `/etc/bao/unseal/key`.
        * Injects a `seal "static"` stanza into `config.hcl`, for example:
          ```hcl
          seal "static" {
            current_key    = "file:///etc/bao/unseal/key"
            current_key_id = "operator-generated-v1"
          }
          ```
      * **External KMS Seal:** If `spec.unseal.type` is set to `"awskms"`, `"gcpckms"`, `"azurekeyvault"`, or `"transit"`:
        * Does NOT create the `<cluster>-unseal-key` Secret.
        * Renders a `seal "<type>"` block with options from `spec.unseal.options`.
        * If `spec.unseal.credentialsSecretRef` is provided, mounts the credentials Secret at `/etc/bao/seal-creds`.
        * For GCP Cloud KMS (`gcpckms`), sets `GOOGLE_APPLICATION_CREDENTIALS` environment variable pointing to the mounted credentials file.
      * Ensures that the first leader initializes and auto-unseals OpenBao using the configured seal mechanism, and that subsequent restarts remain unsealed automatically.

  * **Image Verification (Supply Chain Security):**
      * If `spec.imageVerification.enabled` is `true`, the operator verifies the container image signature using Cosign before creating or updating the StatefulSet.
      * The verification uses the public key provided in `spec.imageVerification.publicKey`.
      * Verification results are cached in-memory to avoid redundant network calls for the same image digest.
      * **Failure Policy:**
        * `Block` (default): If verification fails, sets `ConditionDegraded=True` with `Reason=ImageVerificationFailed` and blocks StatefulSet updates.
        * `Warn`: If verification fails, logs an error and emits a Kubernetes Event but proceeds with StatefulSet updates.
      * This ensures that only cryptographically verified images are deployed, protecting against compromised registries or man-in-the-middle attacks.

**Reconciliation Semantics**

- Watches `OpenBaoCluster` and all owned resources (StatefulSet, Services, ConfigMaps, Secrets, ServiceAccounts, Ingresses, NetworkPolicies).
- Ensures the rendered `config.hcl` and StatefulSet template are consistent with Spec (including multi-tenant naming conventions).
- Performs image signature verification before StatefulSet creation/updates when `spec.imageVerification.enabled` is `true`.
- Automatically creates NetworkPolicies to enforce cluster isolation:
  - Default deny all ingress traffic.
  - Allow ingress from pods within the same cluster (same pod selector labels).
  - Allow ingress from kube-system namespace (for DNS and system components).
  - Allow ingress from OpenBao operator pods on port 8200 (for health checks, initialization, upgrades).
  - Allow egress to DNS (port 53 UDP/TCP) for service discovery.
  - Allow egress to Kubernetes API server (port 443) for pod discovery (discover-k8s provider).
  - Allow egress to cluster pods on ports 8200 and 8201 for Raft communication.
- Uses controller-runtime patch semantics to apply only necessary changes.
- Updates `Available`/`Degraded` conditions based on StatefulSet readiness.
- **OwnerReferences:** All created resources have `OwnerReferences` pointing to the parent `OpenBaoCluster`, enabling automatic garbage collection when the cluster is deleted. The controller is marked as the owner (`controller: true`).

### 4.2.1 Configuration Merge and Protected Stanzas

To avoid configuration conflicts:

- The Operator **owns and enforces** certain protected stanzas in `config.hcl`, including:
  - `listener "tcp"` (addresses, TLS paths, ports).
  - `storage "raft"` (path, `retry_join` / `auto_join` blocks).
  - `seal` (auto-unseal configuration - type and options are managed by the operator based on `spec.unseal`).
  - `api_addr` and `cluster_addr` (operator-managed based on service configuration).
  - `node_id` (operator-managed via template).
- The Operator uses an **allowlist-based validation approach** for `spec.config`:
  - Only valid OpenBao configuration parameters from the official documentation are allowed.
  - Operator-managed parameters are excluded from the allowlist and cannot be overridden.
  - This approach is more secure than a blocklist, as unknown or newly introduced parameters are rejected by default.
- User-provided scalar-style settings (for example, `ui`, `log_level`, `cache_size`) MAY be provided via `spec.config` as key/value pairs and are merged into the operator-generated template as additional attributes.
- Structured block types (audit devices, plugins, telemetry) are configured via dedicated spec fields (`spec.audit`, `spec.plugins`, `spec.telemetry`) rather than raw strings.
- The merge algorithm is deterministic and idempotent:
  - Start from an operator template containing protected stanzas.
  - Merge user-provided attributes from the allowlist.
  - Render structured blocks (audit, plugins, telemetry) as HCL blocks.
- Validation is enforced via a validating admission webhook:
  - The webhook validates `spec.config` keys against an allowlist of valid OpenBao parameters.
  - Structured blocks (`spec.audit`, `spec.plugins`, `spec.telemetry`) are validated for required fields and format correctness.

### 4.2.2 The InitManager (Cluster Initialization)

**Responsibility:** Automate initial cluster initialization and root token management.

The InitManager handles the bootstrapping of a new OpenBao cluster, including initial Raft leader election and storing the root token securely.

  * **Initialization Workflow:**
    1. **Single-Pod Bootstrap:** During initial cluster creation, the InfrastructureManager starts with 1 replica (regardless of `spec.replicas`) until initialization completes.
    2. **Wait for Container:** The InitManager waits for the pod-0 container to be running (not necessarily ready, since the readiness probe may fail until OpenBao is initialized).
    3. **Check Status:** Query OpenBao via the HTTP health endpoint (`GET /v1/sys/health`) using the per-cluster TLS CA to determine if the cluster is already initialized.
    4. **Initialize:** If not initialized, call the HTTP initialization endpoint (`PUT /v1/sys/init`) against pod-0 using the Operator's in-cluster OpenBao client (no `pods/exec` or CLI dependencies).
    5. **Store Root Token:** Parse the initialization response and store the root token in a per-cluster Secret (`<cluster>-root-token`).
    6. **Mark Initialized:** Set `Status.Initialized = true` to signal the InfrastructureManager to scale up to the desired replica count.

  * **Root Token Secret:**
    * Secret name: `<cluster>-root-token`
    * Key: `token`
    * **Security:** This Secret contains the initial root token generated during `bao operator init`. It grants full administrative access to the OpenBao cluster. RBAC must strictly limit access to this Secret.
    * **Lifecycle:** The root token Secret is created during initialization but is NOT automatically cleaned up during cluster deletion (intentional, to allow recovery scenarios).

**Reconciliation Semantics**

- The InitManager only runs when `Status.Initialized == false`.
- Once `Status.Initialized` is true, the InitManager short-circuits and performs no operations.
- Errors during initialization (e.g., network issues, pod not ready) cause requeues with backoff; the cluster remains at 1 replica until successful initialization.
- The initialization response (which contains sensitive unseal keys and root token) is NEVER logged.

**Security Considerations**

- The root token Secret is a high-value asset. See the Threat Model for detailed security analysis.
- Organizations should consider revoking or rotating the root token after initial setup, per OpenBao security best practices.
- The HTTP init endpoint generates unseal keys that are NOT stored by the Operator (they are only shown in the init response). With static auto-unseal, these keys are not needed for normal operation.

### 4.2.3 Self-Initialization Support

**Responsibility:** Enable opt-in OpenBao self-initialization for declarative cluster bootstrapping.

OpenBao's self-initialization feature (https://openbao.org/docs/configuration/self-init/) allows operators to define initial service state through request-driven initialization that occurs automatically on first server start.

**Configuration Generation:**

When `spec.selfInit.enabled = true`, the ConfigController generates `initialize` stanzas in `config.hcl`:

```hcl
# Example generated config with self-init
initialize "audit" {
  request "enable-stdout-audit" {
    operation = "update"
    path = "sys/audit/stdout"
    data = {
      type = "file"
      options = {
        file_path = "/dev/stdout"
        log_raw = true
      }
    }
  }
}

initialize "auth" {
  request "enable-kubernetes-auth" {
    operation = "update"
    path = "sys/auth/kubernetes"
    data = {
      type = "kubernetes"
    }
  }
}
```

**Key Behavioral Differences:**

| Aspect | Standard Init | Self-Init |
|--------|--------------|-----------|
| Initialization | Operator executes `bao operator init` | OpenBao handles internally |
| Root Token | Stored in `<cluster>-root-token` Secret | Auto-revoked, not available |
| Recovery Keys | Not stored (static auto-unseal) | Not generated |
| Initial Config | Manual post-init | Declarative via `initialize` stanzas |

**Reconciliation Semantics:**

- When `spec.selfInit.enabled = true`:
  - ConfigController injects `initialize` stanzas from `spec.selfInit.requests[]`.
  - InitManager skips HTTP-based initialization.
  - InitManager only monitors for `initialized=true` via the HTTP health endpoint.
  - No root token Secret is created.
  - `Status.SelfInitialized = true` is set after successful initialization.

- Request names must be unique and match regex `^[A-Za-z_][A-Za-z0-9_-]*$`.
- Requests are grouped by category (audit, auth, secrets, etc.) for organizational clarity.
- The order of requests within a group is preserved.

**Security Considerations:**

- Self-init requires an auto-unseal mechanism (static auto-unseal satisfies this).
- The root token is never exposed and is auto-revoked after initialization.
- All requests are subject to audit logging (if configured as the first init request).
- Sensitive data in `spec.selfInit.requests[].data` (e.g., tokens, credentials) should be provided via environment variable or file references where supported.

### 4.2.4 Gateway API Support

**Responsibility:** Provide Kubernetes Gateway API as an alternative to Ingress for external access.

The Gateway API is a more expressive, extensible, and role-oriented successor to Ingress. The Operator supports creating `HTTPRoute` resources that route traffic through user-managed `Gateway` resources.

**Resource Model:**

```
Gateway (user-managed)
    |
    v
HTTPRoute (operator-managed: <cluster>-httproute)
    |
    v
Service (<cluster>-public or <cluster>)
    |
    v
OpenBao Pods
```

**Reconciliation Logic:**

When `spec.gateway.enabled = true`, the InfrastructureManager reconciles Gateway API resources as part of its normal flow:

1. **Ensure HTTPRoute:**
   - If Gateway API CRDs are not installed (no `HTTPRoute` kind), log an informational message and skip HTTPRoute reconciliation.
   - If Gateway is disabled (`spec.gateway == nil` or `spec.gateway.enabled == false`), delete any existing HTTPRoute and return.
   - If the Gateway configuration is incomplete (for example, empty `spec.gateway.hostname` or `spec.gateway.gatewayRef.name`), delete any existing HTTPRoute and return without creating a new one.
   - Otherwise, create or update an `HTTPRoute` with:
     - Name: `<cluster>-httproute`
     - Namespace: the `OpenBaoCluster` namespace
     - `parentRefs`: pointing to the configured Gateway (`spec.gateway.gatewayRef`)
     - `hostnames`: `spec.gateway.hostname`
     - `rules`: a single rule routing to the OpenBao public Service backend on port `8200` with a `PathPrefix` match on `spec.gateway.path` (default `/`).

2. **Ensure Gateway CA ConfigMap:**
   - Maintain a CA `ConfigMap` named `<cluster>-tls-ca` in the `OpenBaoCluster` namespace containing the CA certificate in the `ca.crt` key.
   - Keep the `ConfigMap` in sync with the CA Secret of the same name.
   - Delete the `ConfigMap` when Gateway support is disabled for the cluster. The `ConfigMap` also has an `OwnerReference` to the `OpenBaoCluster`, so it is garbage-collected when the cluster is deleted.

3. **Update TLS SANs:**
   - Add `spec.gateway.hostname` to the server certificate SANs when Gateway API is enabled and a non-empty hostname is configured.
   - Trigger certificate regeneration if the hostname set changes.

**Example HTTPRoute (generated):**

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: prod-cluster-httproute
  namespace: security
spec:
  parentRefs:
    - name: main-gateway
      namespace: gateway-system
  hostnames:
    - "bao.example.com"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: prod-cluster-public
          port: 8200
```

**TLS Considerations:**

- Gateway API separates TLS termination concerns from routing.
- The Gateway resource (user-managed) handles TLS termination.
- For end-to-end TLS, the Gateway should be configured with `BackendTLSPolicy` or equivalent to trust the OpenBao CA.
- When Gateway API is enabled, the Operator maintains both a CA Secret and a CA ConfigMap named `<cluster>-tls-ca` that Gateway implementations (for example, Traefik) can reference for backend TLS validation.
- The Operator ensures the hostname is in the server certificate SANs regardless of where TLS terminates.

**Comparison with Ingress:**

| Feature | Ingress | Gateway API |
|---------|---------|-------------|
| Resource | `Ingress` | `HTTPRoute` + `Gateway` |
| TLS Config | Per-Ingress | Per-Gateway (shared) |
| Multi-tenant | Limited | First-class support |
| Backend Config | Annotations | `BackendTLSPolicy` |
| Portability | Varies by controller | Standardized |

### 4.3 The UpgradeManager (The State Machine)

**Responsibility:** Safe rolling updates with Raft-aware leader handling.

#### 4.3.1 OpenBao API Client

The UpgradeManager requires authenticated access to OpenBao's system endpoints for health checks and leader step-down operations.

**Client Configuration:**

```go
type OpenBaoClient struct {
    // HTTPClient with TLS configuration trusting the cluster CA.
    httpClient *http.Client
    // BaseURL is the OpenBao API endpoint (e.g., https://pod-0.cluster.namespace.svc:8200).
    baseURL string
    // Token for authentication (from root token Secret or dedicated operator token).
    token string
}
```

**Authentication:**

- **Primary Method:** The Operator reads the root token from the `<cluster>-root-token` Secret.
- **Alternative (Self-Init Clusters):** For clusters using self-initialization (no root token), users MUST configure a dedicated operator token via `spec.selfInit.requests[]` with the following policy:

```hcl
# Minimal policy for operator upgrade operations
path "sys/health" {
  capabilities = ["read"]
}
path "sys/step-down" {
  capabilities = ["update"]
}
path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}
```

**TLS Configuration:**

- The client trusts the CA certificate from `<cluster>-tls-ca` Secret.
- Client certificates are NOT required (server-only TLS verification).
- The client validates server certificates against the expected SANs.

**Connection Management:**

- Connections are established per-pod using the predictable DNS name (`<pod>.<cluster>.<namespace>.svc`).
- Connection timeout: 5 seconds.
- Request timeout: 10 seconds (configurable via context).

#### 4.3.2 Version Validation and Upgrade Policies

**Semantic Version Comparison:**

The UpgradeManager uses semantic versioning (semver) to validate version changes:

- Versions MUST be valid semver strings (e.g., `2.4.0`, `2.4.1-rc1`).
- Invalid versions cause a `Degraded=True` condition with `Reason=InvalidVersion`.

**Version Change Policies:**

| Change Type | Behavior | Configurable |
|-------------|----------|--------------|
| Patch upgrade (2.4.0 → 2.4.1) | Allowed | No |
| Minor upgrade (2.4.0 → 2.5.0) | Allowed | No |
| Major upgrade (2.x → 3.x) | Allowed with warning | No |
| Minor skip (2.4.0 → 2.6.0) | Allowed with warning | No |
| Downgrade (2.5.0 → 2.4.0) | Blocked by default | `spec.allowDowngrade` (future) |

**Pre-upgrade Validation:**

Before starting an upgrade, the UpgradeManager verifies:

1. All pods are Ready (`ReadyReplicas == Spec.Replicas`).
2. Quorum is healthy (majority of pods are unsealed and joined).
3. A single leader can be identified.
4. No other upgrade is in progress (`Status.Upgrade == nil`).
5. Target version is valid and satisfies upgrade policies.

If any check fails, the upgrade is blocked and `Degraded=True` is set with a descriptive reason.

#### 4.3.3 State Machine Logic

**Detection and Initialization:**

1. **Detect Drift:** `Spec.Version` != `Status.CurrentVersion` AND `Status.Upgrade == nil`.
2. **Pre-upgrade Snapshot:** If `spec.backup.preUpgradeSnapshot == true`, trigger a backup and wait for completion. If backup fails, block upgrade and set `Degraded=True` with `Reason=PreUpgradeBackupFailed`.
3. **Initialize Upgrade State:**
   - Create `Status.Upgrade` with `TargetVersion`, `FromVersion`, `StartedAt`.
   - Set `Status.Phase = Upgrading`.
   - Set `Upgrading=True` condition.
4. **Lock StatefulSet:** Patch StatefulSet `updateStrategy.rollingUpdate.partition` to `Replicas` (pauses all updates).

**Pod-by-Pod Update (Reverse Ordinal):**

For each pod from highest ordinal to 0:

1. **Check Leadership:**
   - Query `GET /v1/sys/health` on target pod.
   - If pod is leader (`standby: false`):
     - Call `PUT /v1/sys/step-down` with empty body.
     - Wait up to `StepDownTimeout` (default: 30s) for leadership to transfer.
     - Record `Status.Upgrade.LastStepDownTime`.
     - If timeout: Set `Degraded=True` with `Reason=StepDownTimeout`, halt upgrade.

2. **Allow Pod Update:**
   - Decrement partition to allow Kubernetes to update the target pod.
   - Patch StatefulSet with new partition value.

3. **Wait for Pod Ready:**
   - Wait for pod to be recreated and reach `Ready` state.
   - Timeout: `PodReadyTimeout` (default: 5 minutes).

4. **Wait for OpenBao Health:**
   - Poll `GET /v1/sys/health` until `initialized: true` AND `sealed: false`.
   - Polling interval: 5 seconds.
   - Timeout: `HealthCheckTimeout` (default: 2 minutes).

5. **Wait for Raft Sync:**
   - Compare pod's `raft_committed_index` with leader's index.
   - Wait until pod is within acceptable lag (default: 100 entries).
   - Timeout: `RaftSyncTimeout` (default: 2 minutes).

6. **Record Progress:**
   - Add pod ordinal to `Status.Upgrade.CompletedPods`.
   - Update `Status.Upgrade.CurrentPartition`.

**Finalization:**

1. Update `Status.CurrentVersion` to match `Spec.Version`.
2. Clear `Status.Upgrade` (set to nil).
3. Set `Status.Phase = Running`.
4. Set `Upgrading=False` condition with `Reason=UpgradeComplete`.

#### 4.3.4 Timeout Configuration

| Timeout | Default | Description |
|---------|---------|-------------|
| `StepDownTimeout` | 30s | Max time to wait for leader step-down |
| `PodReadyTimeout` | 5m | Max time to wait for pod to become Ready |
| `HealthCheckTimeout` | 2m | Max time to wait for OpenBao to become healthy |
| `RaftSyncTimeout` | 2m | Max time to wait for Raft index to sync |
| `TotalUpgradeTimeout` | Replicas × 10m | Max total upgrade duration |

Timeouts are enforced via context deadlines. When a timeout is exceeded:
- The upgrade halts at the current state.
- `Degraded=True` condition is set with appropriate reason.
- The partition remains at the current value (no further pods updated).
- Manual intervention may be required.

#### 4.3.5 Resumability and Restart Handling

Upgrades are designed to survive Operator restarts:

**State Persistence:**

All upgrade state is stored in `Status.Upgrade`:
- `TargetVersion`: The version being upgraded to.
- `FromVersion`: The version being upgraded from.
- `StartedAt`: When the upgrade began.
- `CurrentPartition`: Current StatefulSet partition.
- `CompletedPods`: Which pods have been upgraded.

**Resume Logic:**

On Operator restart (or requeue), if `Status.Upgrade != nil`:
1. Verify upgrade is still needed (`Spec.Version == Status.Upgrade.TargetVersion`).
2. If user changed `Spec.Version` mid-upgrade, clear `Status.Upgrade` and start fresh.
3. Otherwise, resume from `Status.Upgrade.CurrentPartition`.
4. Re-verify health of already-upgraded pods before continuing.

#### 4.3.6 Rollback Strategy

**Automated Rollback: NOT Supported**

The Operator does NOT automatically roll back failed upgrades. Reasons:
- Rollback may not be safe for all version transitions.
- Data migrations may be irreversible.
- Operator cannot distinguish between "safe to rollback" and "must go forward."

**On Upgrade Failure:**

1. Upgrade halts at current state.
2. `Degraded=True` condition set with detailed reason.
3. `Status.Upgrade` preserved (showing partial progress).
4. StatefulSet partition remains set (preventing further updates).

**Manual Rollback Procedure:**

If rollback is necessary:

1. Set `spec.paused = true` to prevent reconciliation.
2. Assess cluster state (which pods are on new version vs old).
3. If safe to proceed backward:
   - Update `spec.version` and `spec.image` to previous version.
   - Set `spec.paused = false`.
   - Operator will detect "upgrade" to older version and proceed.
4. If NOT safe (data migration occurred):
   - Contact OpenBao support or consult documentation.
   - May require restore from backup.

**Mixed-Version Handling:**

If pods are running mixed versions (partial upgrade):
- The cluster may still be operational (Raft tolerates version differences for limited time).
- `Degraded=True` condition indicates mixed state.
- Completing the upgrade forward is generally the safest path.

#### 4.3.7 Reconciliation Semantics

- Only one upgrade per `OpenBaoCluster` may run at a time (serialized via `Status.Upgrade`).
- Transient OpenBao API errors cause retries with backoff; persistent failures result in `Upgrading=False` and `Degraded=True` conditions.
- The Operator MUST NOT start a new upgrade while `Status.Upgrade != nil`.
- Pause integration: If `spec.paused` becomes true mid-upgrade, the upgrade halts but `Status.Upgrade` is preserved. Resume by setting `spec.paused = false`.

#### 4.3.8 Safety Checks and Split-Brain Handling

- The UpgradeManager MUST verify that a healthy leader can be determined before proceeding:
  - If the Operator cannot determine a single, consistent leader (e.g., due to API timeouts, split-brain symptoms, or quorum loss), it MUST halt the upgrade.
  - In this case, it sets `Upgrading=False` and `Degraded=True` Conditions with a clear reason (e.g., `Reason=LeaderUnknown`), and leaves the StatefulSet partition unchanged.
- The UpgradeManager MUST NOT attempt step-down or pod updates when cluster health checks indicate loss of quorum or when multiple pods claim leadership.
- Quorum check: At least `(Replicas / 2) + 1` pods must be healthy before and after each pod update.

-----

### 4.4 The BackupManager (Snapshots to Object Storage)

**Responsibility:** Schedule and execute Raft snapshots, stream them to object storage, and manage retention.

#### 4.4.1 Backup Authentication

The BackupManager requires authenticated access to OpenBao's snapshot endpoint (`GET /v1/sys/storage/raft/snapshot`).

**Authentication Strategy:**

The operator supports two authentication methods for backup operations, in order of preference:

1. **Kubernetes Auth (Preferred):** Uses ServiceAccount tokens that are automatically rotated by Kubernetes.
   - Configured via `spec.backup.kubernetesAuthRole`
   - Requires a Kubernetes Auth role in OpenBao that binds to the backup ServiceAccount
   - The operator automatically creates `<cluster-name>-backup-serviceaccount` for backup Jobs
   - More secure as tokens are automatically rotated and have limited lifetime

2. **Static Token (Fallback):** Uses a long-lived token from a Kubernetes Secret.
   - **All Clusters:** Must be configured via `spec.backup.tokenSecretRef` pointing to a Secret with a backup-capable token
   - Root tokens are not used for backup operations. Users must create a dedicated backup token with minimal permissions.

**Required OpenBao Policy for Backups:**

```hcl
# Minimal policy for backup operations
path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}
```

**Kubernetes Auth Configuration Example:**

```yaml
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
      # The ServiceAccount name is <cluster-name>-backup-serviceaccount
      - name: create-backup-kubernetes-role
        operation: update
        path: auth/kubernetes/role/backup
        data:
          bound_service_account_names: <cluster-name>-backup-serviceaccount
          bound_service_account_namespaces: <cluster-namespace>
          policies: backup
          ttl: 1h
  backup:
    kubernetesAuthRole: backup  # Reference to the role created above
```

**Static Token Configuration Example (Self-Init):**

```yaml
spec:
  selfInit:
    enabled: true
    requests:
      - name: enable-approle
        operation: update
        path: sys/auth/approle
        data:
          type: approle
      - name: create-backup-policy
        operation: update
        path: sys/policies/acl/backup
        data:
          policy: |
            path "sys/storage/raft/snapshot" {
              capabilities = ["read"]
            }
      - name: create-backup-role
        operation: update
        path: auth/approle/role/operator-backup
        data:
          token_policies: ["backup"]
          token_ttl: "1h"
  backup:
    tokenSecretRef:
      name: backup-token-secret
      namespace: openbao-operator-system
```

**ServiceAccount Management:**

- The operator automatically creates `<cluster-name>-backup-serviceaccount` when backups are enabled
- This ServiceAccount is used by backup Jobs for Kubernetes Auth
- The ServiceAccount token is automatically mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`
- For Kubernetes Auth to work, the OpenBaoCluster's ServiceAccount must have the `system:auth-delegator` ClusterRole (for token verification)

**Token Handling:**

- **Kubernetes Auth:** The backup executor reads the ServiceAccount token and authenticates to OpenBao using the configured role
- **All Clusters (Static Token):** Users MUST configure `spec.backup.tokenSecretRef` pointing to a Secret with a backup-capable token. Root tokens are not used for backup operations.
- If no authentication method is available (self-init cluster without `kubernetesAuthRole` or `tokenSecretRef`),
  backups are skipped with `BackingUp=False` condition and `Reason=NoBackupToken`

#### 4.4.2 Object Storage Client

The BackupManager uses a cloud-agnostic object storage interface compatible with S3-style APIs.

**SDK Choice:**

- Primary: AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2`)
- This SDK supports S3-compatible endpoints (MinIO, GCS via S3 compatibility, Ceph, etc.)

**Client Configuration:**

```go
type ObjectStorageClient struct {
    s3Client  *s3.Client
    bucket    string
    pathPrefix string
}
```

**Authentication Methods:**

The BackupManager supports multiple authentication methods based on `spec.backup.target.credentialsSecretRef`:

| Method | Secret Keys | Description |
|--------|-------------|-------------|
| Static Credentials | `accessKeyId`, `secretAccessKey` | Direct credentials |
| Session Token | `accessKeyId`, `secretAccessKey`, `sessionToken` | Temporary credentials |
| Workload Identity | (none - Secret not required) | IRSA, GKE Workload Identity, Azure Workload Identity |
| Instance Metadata | (none - Secret not required) | EC2 Instance Profile, GCE Metadata |

**Credentials Secret Structure:**

When `spec.backup.target.credentialsSecretRef` is provided, the Secret MAY contain:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: backup-credentials
  namespace: security
type: Opaque
stringData:
  # Required for static credentials
  accessKeyId: "AKIAIOSFODNN7EXAMPLE"
  secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  # Optional
  sessionToken: "..."  # For temporary credentials
  region: "us-west-2"  # Override region detection
```

If no credentials Secret is provided, the client attempts instance-level credentials via the default credential chain.

**TLS Configuration:**

- By default, verify server TLS certificates.
- For testing with self-signed certificates (e.g., MinIO), users can add CA certificates via the credentials Secret (`caCert` key) or disable verification (NOT recommended for production).

**Upload Strategy:**

- Snapshots < 100MB: Single PUT request.
- Snapshots >= 100MB: Multipart upload with 10MB parts.
- Upload timeout: 30 minutes (configurable).

#### 4.4.3 Backup Naming Convention

Backups are named predictably to enable easy discovery, sorting, and retention management.

**Object Key Format:**

```
<pathPrefix>/<namespace>/<cluster>/<timestamp>-<short-uuid>.snap
```

**Components:**

| Component | Format | Example |
|-----------|--------|---------|
| `pathPrefix` | User-configured | `prod/` |
| `namespace` | Kubernetes namespace | `security` |
| `cluster` | OpenBaoCluster name | `prod-cluster` |
| `timestamp` | RFC3339 in UTC | `2025-01-15T03-00-00Z` |
| `short-uuid` | 8 hex characters | `a1b2c3d4` |
| Extension | Fixed | `.snap` |

**Example Full Path:**

```
prod/security/prod-cluster/2025-01-15T03-00-00Z-a1b2c3d4.snap
```

**Rationale:**

- Prefix allows per-tenant isolation in shared buckets.
- Namespace/cluster path enables per-cluster retention.
- Timestamp enables chronological sorting.
- Short UUID prevents collisions (e.g., rapid manual triggers).
- `.snap` extension identifies the file type.

#### 4.4.4 Backup Scheduling

The BackupManager uses an in-process scheduler to trigger backups based on cron expressions.

**Scheduler Implementation:**

- Uses `github.com/robfig/cron/v3` for cron parsing and scheduling.
- One scheduler instance per `OpenBaoCluster` (managed by the Reconciler).
- Schedules are recalculated on every reconciliation.

**Schedule Persistence:**

- `Status.Backup.NextScheduledBackup` stores the next scheduled time.
- On Operator restart, the next schedule is recalculated from the cron expression.
- If a scheduled backup was missed, the BackupManager checks if the last backup is older than one schedule interval; if so, it triggers immediately.

**Missed Schedule Handling:**

```
if Status.Backup.LastBackupTime == nil:
    trigger immediately
else if Now - LastBackupTime > 2 * ScheduleInterval:
    trigger immediately
else:
    wait for next scheduled time
```

**Minimum Schedule Interval:**

To prevent resource exhaustion, the Operator warns (via `Degraded=True` condition with `Reason=BackupScheduleTooFrequent`) if the schedule interval is less than 1 hour. Schedules more frequent than 15 minutes are rejected by the validating webhook.

#### 4.4.5 Backup Execution Flow

Backups are executed using Kubernetes Jobs that run the `bao-backup` executor container. This design keeps the operator stateless and allows backups to run independently.

1. **Pre-flight Checks:**
   - Verify cluster is healthy (`Phase == Running`).
   - Verify no upgrade is in progress (`Status.Upgrade == nil`).
   - Verify no other backup is in progress (`BackingUp` condition is False).
   - Verify authentication is configured (Kubernetes Auth role or token Secret).
   - If checks fail, skip backup and log reason.

2. **Ensure Backup ServiceAccount:**
   - Create or update `<cluster-name>-backup-serviceaccount` if it doesn't exist.
   - This ServiceAccount is used by backup Jobs for Kubernetes Auth.

3. **Create Backup Job:**
   - Create a Kubernetes Job with a unique name: `backup-<cluster-name>-<timestamp>`
   - Job runs the backup executor image (`spec.backup.executorImage`)
   - Job Pod uses the backup ServiceAccount
   - Job Pod mounts:
     - TLS CA certificate from `<cluster-name>-tls-ca` Secret
     - Storage credentials Secret (if configured)
     - OpenBao token Secret (if using static token auth)
   - Job environment variables include:
     - Cluster information (namespace, name, replicas)
     - Storage configuration (endpoint, bucket, pathPrefix)
     - Authentication configuration (`BACKUP_KUBERNETES_AUTH_ROLE` or token Secret reference)

4. **Set Backup State:**
   - Set `BackingUp=True` condition.
   - Set `Status.Phase = BackingUp` (if not during upgrade).

5. **Backup Executor Workflow:**
   The `bao-backup` executor performs the following steps:
   - **Load Configuration:** Read environment variables and mounted files
   - **Authenticate:**
     - If `BACKUP_KUBERNETES_AUTH_ROLE` is set: Use Kubernetes Auth with ServiceAccount token
     - Otherwise: Use static token from mounted Secret
   - **Discover Leader:** Query all pods to find the Raft leader
   - **Generate Backup Key:** Create deterministic object key with timestamp and UUID
   - **Stream Snapshot:**
     - Open `GET /v1/sys/storage/raft/snapshot` from leader
     - Pipe response body directly to object storage upload
     - Track bytes transferred for size reporting
     - Timeout: 30 minutes
   - **Verify Upload:**
     - After upload, perform HEAD request to verify object exists
     - Verify object has non-zero size
   - **Exit:** Return exit code 0 on success, non-zero on failure

6. **Process Job Result:**
   - Monitor Job status (Active, Succeeded, Failed)
   - On success:
     - Set `Status.Backup.LastBackupTime = Now`
     - Calculate and set `Status.Backup.NextScheduledBackup`
     - Reset `Status.Backup.ConsecutiveFailures = 0`
     - Clear `Status.Backup.LastFailureReason`
     - Set `BackingUp=False` condition with `Reason=BackupSucceeded`
   - On failure:
     - Increment `Status.Backup.ConsecutiveFailures`
     - Set `Status.Backup.LastFailureReason` with error description
     - Set `BackingUp=False` condition with `Reason=BackupFailed`

7. **Apply Retention:**
   - If retention policy is configured, delete old backups (see 4.4.6).
   - Retention runs after successful backup completion.

8. **Job Cleanup:**
   - Jobs have TTL of 1 hour after completion (`TTLSecondsAfterFinished`)
   - Kubernetes automatically cleans up completed/failed Jobs

**On Failure:**

- Increment `Status.Backup.ConsecutiveFailures`.
- Set `Status.Backup.LastFailureReason` with error description.
- Set `BackingUp=False` condition with `Reason=BackupFailed`.
- Increment `openbao_backup_failure_total` metric.
- Restore `Status.Phase` to previous value.

#### 4.4.6 Retention Policy Enforcement

When `spec.backup.retention` is configured, the BackupManager deletes old backups after successful new backup upload.

**Retention Logic:**

```
1. List all objects matching <pathPrefix>/<namespace>/<cluster>/*
2. Sort by timestamp (newest first)
3. If MaxCount > 0:
     Delete all objects beyond MaxCount
4. If MaxAge is set:
     Delete all objects older than Now - MaxAge
```

**Implementation Notes:**

- Retention is applied AFTER successful backup upload.
- Deletion failures are logged but do not fail the backup.
- Retention uses ListObjectsV2 pagination for large backup sets.
- Deletion is batched (up to 1000 objects per DeleteObjects call).

#### 4.4.7 Concurrent Backup Prevention

The BackupManager prevents concurrent backup operations:

**Within Same Cluster:**

- Check `BackingUp` condition before starting.
- If `BackingUp=True`, skip and requeue.
- Use optimistic locking via Status updates.

**Backup During Upgrade:**

- If `Status.Upgrade != nil`, skip scheduled backups.
- Log: "Skipping scheduled backup: upgrade in progress."
- Exception: Pre-upgrade backups (triggered BY the UpgradeManager) proceed.

**Manual Trigger Support (Future):**

A future enhancement may allow manual backup triggers via annotation:
- `openbao.org/backup-now: "true"` triggers immediate backup.
- Annotation is cleared after backup completes or fails.

#### 4.4.8 Cloud-Agnostic Design

- The backup target is configured via generic fields (endpoint URL, bucket/container name, path prefix).
- Credentials are provided via Kubernetes Secrets or workload identity, independent of specific cloud providers.
- Cluster operators MAY additionally constrain `BackupTarget` configuration via:
  - Admission control (e.g., a Validating Webhook or external policy engine) that enforces an allowlist of endpoints, buckets, or prefixes.
  - Namespace-scoped RBAC that limits who can modify backup configuration.

All external calls performed by the BackupManager (OpenBao API, Kubernetes API, and object storage SDKs) MUST accept a `context.Context`, use bounded timeouts, and participate in the shared error-handling and rate-limiting strategy described below.

-----

### 4.5 Cross-Cutting Error Handling & Rate Limiting

All Managers (Cert, Infrastructure, Upgrade, Backup) share the following error-handling and reconciliation semantics:

- External operations (Kubernetes API, OpenBao API, object storage SDKs) MUST:
  - Accept `context.Context` as the first parameter.
  - Use time-bounded contexts so that calls respect reconciliation deadlines and cancellation.
- Transient errors (e.g., network timeouts, optimistic concurrency conflicts) are retried via the controller-runtime workqueue rate limiter rather than custom `time.Sleep` loops.
- Per-controller `MaxConcurrentReconciles` values are configured to:
  - Prevent a single noisy `OpenBaoCluster` from starving others.
  - Support at least 10 concurrent `OpenBaoCluster` resources under normal load, in line with the non-functional requirements.
- Persistent failures MUST:
  - Surface as typed `Conditions` (e.g., `ConditionDegraded`, `ConditionUpgrading`, `ConditionBackingUp`, `ConditionEtcdEncryptionWarning`) with clear reasons and messages.
  - Avoid hot reconcile loops by relying on exponential backoff from the rate limiter.
- Audit logging:
  - Critical operations (initialization, leader step-down) MUST emit structured audit logs tagged with `audit=true` and `event_type` for security monitoring.

-----

## 5\. Disaster Recovery Design

### 5.1 Backup Workflow

We utilize Kubernetes Jobs with a dedicated backup executor container to stream data from OpenBao directly to object storage, keeping the Operator stateless and low-memory.

1.  **Scheduler:** Standard Kubernetes `CronJob` logic inside the Operator process.
2.  **Job Creation:** Operator creates a Kubernetes Job with the backup executor image.
3.  **Execution (in Job container):**
      * Authenticate to OpenBao using Kubernetes Auth (preferred) or static token (fallback).
      * Locate Leader (query OpenBao API health endpoints).
      * Open Stream: `GET /sys/storage/raft/snapshot`.
      * Open Upload Stream: object storage SDK `Upload()`.
      * Pipe Reader -\> Writer (no in-memory buffering).
      * Verify upload with HEAD request.
4.  **Authentication:**
      * **Kubernetes Auth (Preferred):** Uses ServiceAccount token mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`
      * **Static Token (Fallback):** Uses token from mounted Secret
      * Operator automatically creates `<cluster-name>-backup-serviceaccount` for backup Jobs
5.  **Encryption:** We rely on the object storage provider's server-side encryption or OpenBao's native snapshot encryption (if enterprise/configured), and we RECOMMEND:
      * Per-tenant or per-cluster buckets/prefixes for isolation.
      * Least-privilege credentials that can only write to the intended location.
      * Kubernetes Auth for automatic token rotation and better security.

### 5.2 Deletion Policy and Data Retention

The `DeletionPolicy` in `OpenBaoCluster.Spec` governs what happens when the CR is deleted:

- `Retain`: The Operator deletes Kubernetes control-plane resources (StatefulSet, Services, ConfigMaps, Secrets related to the cluster) but retains PVCs and external backups.
- `DeletePVCs`: The Operator deletes the StatefulSet and PVCs associated with the cluster while retaining external backups.
- `DeleteAll`: The Operator deletes the StatefulSet, PVCs, and attempts to delete associated backup objects from the configured object storage.

Finalizers on the `OpenBaoCluster` resource ensure that cleanup is performed before the resource is fully removed from the API server.

### 5.3 Manual Restore Workflow (Out of Scope for Automation)

Automated restores are explicitly a non-goal for v0.1, but we must support a safe **manual** restore path for operators.

High-level manual restore procedure:

1.  **Enter Maintenance Mode:**
      * User sets `spec.paused = true` on the `OpenBaoCluster`.
      * All reconcilers for that cluster stop mutating resources (no scale-up/scale-down, no upgrades, no config changes).
2.  **Quiesce the Cluster:**
      * User scales the StatefulSet down to 0 replicas and ensures no OpenBao pods are running.
3.  **Restore Data:**
      * User provisions new PVCs or restores snapshot data into the existing PVCs (platform-specific).
      * User starts a one-off Pod or Job with the OpenBao binary and performs `raft snapshot restore` from the desired backup object (following OpenBao documentation).
4.  **Bring the Cluster Back:**
      * User scales the StatefulSet back up to the desired replica count.
      * OpenBao pods start, using the restored Raft data and the existing static auto-unseal key.
5.  **Exit Maintenance Mode:**
      * User sets `spec.paused = false` to resume normal reconciliation.

The Operator MUST NOT attempt to override storage or configuration while `spec.paused=true`, allowing this manual procedure to complete without interference.

-----

## 6\. Security & RBAC

The Operator requires elevated privileges to function as a Supervisor.

**ClusterRole Requirements:**

  * `secrets`: **Create, Update, Get, List, Watch** (For TLS management).
  * `statefulsets`: **All** (For partitioning and scaling).
  * `pods`: **Get, List, Watch** (To identify leaders).
  * `pods/exec`: **Create** (For SIGHUP/Hot Reloads).
  * `configmaps`: **Create, Update** (For `config.hcl`).
  * `networkpolicies`: **Create, Update, Get, List, Watch, Delete** (For automatic cluster isolation).

**Security Context:**

  * Operator runs as non-root.
  * TLS Keys generated by the Operator never leave the cluster (stored in Secrets).

**Auto-Unseal Security Architecture:**

- **Static Seal (Default):** The Operator uses OpenBao's static auto-unseal mechanism and manages a per-cluster unseal key stored in a Kubernetes Secret (e.g., `<cluster>-unseal-key`). The Root of Trust for OpenBao data is anchored in Kubernetes Secrets (and ultimately etcd). If an attacker can read Secrets in the OpenBao namespace, they can obtain the unseal key and decrypt OpenBao data.
  - Mitigations and expectations:
    - Kubernetes etcd encryption at rest SHOULD be enabled and properly configured.
    - RBAC MUST strictly limit access to Secrets in namespaces where OpenBao clusters run.
    - Platform teams SHOULD apply additional controls such as node security hardening and audit logging for Secret access.
    - Rotation of the static auto-unseal key is **not automated** in v0.1; any key rotation procedure is a manual, carefully planned operation that requires re-encryption of data and is out of scope for this version.

- **External KMS Seal:** When `spec.unseal.type` is set to an external KMS provider (`awskms`, `gcpckms`, `azurekeyvault`, `transit`), the Root of Trust shifts from Kubernetes Secrets to the cloud provider's KMS service. The operator does not generate or manage the master key for these clusters, improving security posture by removing the operator from the key management path. Credentials for the KMS provider can be provided via `spec.unseal.credentialsSecretRef` or through workload identity mechanisms (IRSA for AWS, GKE Workload Identity for GCP).

**Root Token Secret:**

- During cluster initialization, the Operator stores the initial root token in a per-cluster Secret (`<cluster>-root-token`).
- The root token grants **full administrative access** to the OpenBao cluster and is a critical security asset.
- Security requirements:
  - RBAC MUST strictly limit access to the root token Secret. Only cluster administrators and the Operator ServiceAccount should have read access.
  - Platform teams SHOULD configure audit logging to monitor access to this Secret.
  - Organizations SHOULD consider revoking or rotating the root token after initial cluster configuration, per OpenBao security best practices.
  - The root token Secret is NOT automatically deleted when the `OpenBaoCluster` is deleted (intentional, to support recovery scenarios); manual cleanup may be required.
  - The Operator sets an `EtcdEncryptionWarning` condition to remind users that security relies on underlying Kubernetes secret encryption at rest. The operator cannot verify etcd encryption status, so this condition is always set with a message reminding users to ensure etcd encryption is enabled.
- The initialization response (which includes unseal keys) is NEVER logged by the Operator.

Multi-tenancy implications:

- RBAC can be configured to allow namespace-scoped teams to manage only `OpenBaoCluster` resources in their own namespaces.
- The Operator implementation MUST avoid cross-namespace assumptions; all resource names must be fully qualified with namespace.
 - Backup destinations SHOULD be isolated per tenant or per cluster where possible, and backup credentials MUST follow least-privilege principles.

-----

## 7\. Observability & Metrics

The Operator exposes metrics suitable for Prometheus-style scraping and emits structured audit logs for critical operations.

### 7.1 Operator Audit Logging

The Operator emits structured audit logs for critical security-relevant operations, distinct from regular debug/info logs. Audit events are tagged with `audit=true` and `event_type` for easy filtering in log aggregation systems.

**Audit Events:**
- **Cluster Initialization:** `Init`, `InitCompleted`, `InitFailed` - Logged when the operator initializes a new OpenBao cluster
- **Leader Step-Down:** `StepDown`, `StepDownCompleted`, `StepDownFailed` - Logged during upgrade operations when the operator performs leader step-down

All audit events include contextual fields such as `cluster_namespace`, `cluster_name`, and operation-specific details (e.g., `pod`, `target_version`, `from_version` for upgrades).

**Example Audit Log Entry:**
```
{
  "audit": "true",
  "event_type": "StepDown",
  "cluster_namespace": "security",
  "cluster_name": "prod-cluster",
  "pod": "prod-cluster-0",
  "target_version": "2.2.0",
  "from_version": "2.1.0"
}
```

### 7.2 Core Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `openbao_cluster_ready_replicas` | Gauge | `namespace`, `name` | Number of Ready replicas |
| `openbao_cluster_phase` | Gauge | `namespace`, `name`, `phase` | Current cluster phase (1 = active phase) |
| `openbao_reconcile_duration_seconds` | Histogram | `namespace`, `name`, `controller` | Reconciliation duration |
| `openbao_reconcile_errors_total` | Counter | `namespace`, `name`, `controller`, `reason` | Reconciliation errors |

### 7.3 Upgrade Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `openbao_upgrade_status` | Gauge | `namespace`, `name` | Upgrade status: 0=none, 1=running, 2=success, 3=failed |
| `openbao_upgrade_duration_seconds` | Histogram | `namespace`, `name` | Total upgrade duration |
| `openbao_upgrade_pod_duration_seconds` | Histogram | `namespace`, `name`, `pod` | Per-pod upgrade duration |
| `openbao_upgrade_stepdown_total` | Counter | `namespace`, `name` | Total leader step-down operations |
| `openbao_upgrade_stepdown_failures_total` | Counter | `namespace`, `name` | Failed leader step-down attempts |
| `openbao_upgrade_in_progress` | Gauge | `namespace`, `name` | 1 if upgrade in progress, 0 otherwise |

### 7.4 Backup Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `openbao_backup_last_success_timestamp` | Gauge | `namespace`, `name` | Unix timestamp of last successful backup |
| `openbao_backup_last_duration_seconds` | Gauge | `namespace`, `name` | Duration of last backup in seconds |
| `openbao_backup_last_size_bytes` | Gauge | `namespace`, `name` | Size of last backup in bytes |
| `openbao_backup_success_total` | Counter | `namespace`, `name` | Total successful backups |
| `openbao_backup_failure_total` | Counter | `namespace`, `name` | Total backup failures |
| `openbao_backup_consecutive_failures` | Gauge | `namespace`, `name` | Current consecutive failure count |
| `openbao_backup_in_progress` | Gauge | `namespace`, `name` | 1 if backup in progress, 0 otherwise |
| `openbao_backup_retention_deleted_total` | Counter | `namespace`, `name` | Backups deleted by retention policy |

### 7.4 TLS Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `openbao_tls_cert_expiry_timestamp` | Gauge | `namespace`, `name`, `type` | Unix timestamp of certificate expiry |
| `openbao_tls_rotation_total` | Counter | `namespace`, `name` | Total certificate rotations |

### 7.5 Logging

Structured logs include:
- `namespace`: Kubernetes namespace of the `OpenBaoCluster`.
- `name`: Name of the `OpenBaoCluster`.
- `controller`: Name of the controller/manager (e.g., `CertManager`, `UpgradeManager`).
- `reconcile_id`: Unique correlation ID per reconciliation loop.

**Log Levels:**

| Level | Use Case |
|-------|----------|
| Error | Failures requiring attention (backup failed, upgrade halted) |
| Warn | Recoverable issues (transient API errors, approaching cert expiry) |
| Info | Normal operations (backup completed, upgrade started) |
| Debug | Detailed operations (API calls, state transitions) |

**Sensitive Data:**

The following MUST NEVER be logged:
- Root tokens or any authentication tokens.
- Unseal keys or key material.
- Backup contents or snapshot data.
- Credentials for object storage.

### 7.6 Metrics for Non-Functional Requirements

These metrics are used to validate and enforce the non-functional requirements:

- `openbao_reconcile_duration_seconds` underpins the target that, under normal load, reconciliations for an `OpenBaoCluster` complete within approximately 30 seconds.
- Per-cluster gauges and counters support running at least 10 concurrent `OpenBaoCluster` resources without unacceptable degradation in reconcile latency.
- Operator CPU and memory usage for typical cluster sizes MUST be measured during performance testing and documented for operators.

### 7.7 Prometheus Integration

For clusters using the Prometheus Operator, we MAY ship example `ServiceMonitor`/`PodMonitor` resources that scrape the Operator's metrics endpoint; however, creation and wiring of these resources is environment-specific and remains the responsibility of platform teams.

**Example ServiceMonitor:**

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: openbao-operator
  namespace: openbao-system
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: openbao-operator
  endpoints:
    - port: metrics
      interval: 30s
```

-----

## 8\. Testing Strategy

To ensure correctness and avoid regressions, we adopt a layered testing strategy:

- **Unit and Integration Tests (controller-runtime / envtest):**
  - Validate reconciliation logic for each Manager (Cert, Infrastructure, Upgrade, Backup).
  - Cover `spec.paused` semantics (controllers must short-circuit when paused).
  - Verify DeletionPolicy behavior and finalizer handling.
  - Exercise configuration merge and protected-stanza enforcement.

- **End-to-End Tests (kind + KUTTL or Ginkgo):**
  - Bring up a test cluster, deploy the Operator and a real OpenBao image.
  - Scenarios: cluster creation, upgrade (including leader changes), backup scheduling, and manual maintenance workflows.
  - Assertions expressed as CRD updates and expected Status/Pod transitions.

- **Backup Integration (MinIO in CI):**
  - Run a MinIO instance as an S3-compatible endpoint during CI.
  - Validate that the BackupManager streams snapshots end-to-end without buffering to disk and with correct object naming.

These tests are integrated into CI so that changes to the Operator or CRD schema are validated before release.

-----
