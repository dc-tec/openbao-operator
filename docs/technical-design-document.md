This Technical Design Document (TDD) formalizes the architecture for the **OpenBao Supervisor Operator**. It serves as the blueprint for the implementation phase.

-----

# Technical Design Document: OpenBao Supervisor Operator

**Version:** 0.1.0-draft
**Status:** Approved for Development
**Architectural Pattern:** Supervisor / Level 3 Capability (Deep Insights & Lifecycle)

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
    // The Operator will merge this with its own template, but certain
    // protected stanzas (e.g., listener "tcp", storage "raft") are
    // always owned and enforced by the Operator.
    Config   map[string]string `json:"config,omitempty"`

    // Operations
    Backup   *BackupSchedule   `json:"backup,omitempty"`

    // Deletion behavior when the OpenBaoCluster is removed.
    // e.g., "Retain" (keep PVCs and backups), "DeletePVCs", "DeleteAll".
    DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
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

// BackupSchedule defines when and where snapshots are stored.
type BackupSchedule struct {
    // Cron-style schedule, e.g., "0 3 * * *".
    Schedule string `json:"schedule"`
    // Target object storage configuration.
    Target BackupTarget `json:"target"`
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

    // Last successful backup timestamp (if backups are configured).
    LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`

    // Conditions: Available, Upgrading, Degraded, BackingUp, etc.
    Conditions []metav1.Condition `json:"conditions"`
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

-----

## 4\. Detailed Controller Design

The Operator runs a single `OpenBaoCluster` controller which delegates to three specific internal "Managers" to separate concerns.

All controllers MUST honor `spec.paused`:

- When `OpenBaoCluster.Spec.Paused == true`, reconcilers SHOULD short-circuit early and avoid mutating cluster resources, allowing safe manual maintenance (e.g., manual restore).
- Finalizers and deletion handling MAY still proceed to ensure cleanup when the CR is deleted.

### 4.1 The CertManager (TLS Lifecycle)

**Responsibility:** Bootstrap PKI and Rotate Certificates.

  * **Logic Flow:**
    1.  **Reconcile:** Check if `Secret/bao-tls-ca` exists.
    2.  **Bootstrap:** If missing, generate 2048-bit RSA Root CA. Store PEM in Secret.
    3.  **Issue:** Check if `Secret/bao-tls-server` exists and is valid (expiry \> 7 days).
    4.  **Rotate:** If expiry \< 7 days, regenerate using the CA. Update Secret.
    5.  **Trigger:** Compute SHA256 hash of the new cert. Update annotation `openbao.org/tls-hash` on the Pods. This triggers the K8s Kubelet to update the volume mount.
    6.  **Hot Reload:** The Operator detects the annotation change and executes `kill -1 (SIGHUP)` inside the OpenBao container to force a config reload.

**Reconciliation Semantics**

- **Idempotency:** Re-running reconciliation with the same Spec MUST lead to the same Secrets and annotations.
- **Backoff:** Transient errors (e.g., failed Secret update due to concurrent modification) are retried with exponential backoff.
- **Conditions:** Failures update a `TLSReady=False` condition with a clear reason and message.

### 4.2 The InfrastructureManager (Config & StatefulSet)

**Responsibility:** Render configuration and maintain the StatefulSet.

  * **Configuration Strategy:**
      * We do not use a static ConfigMap. We generate it dynamically based on the cluster topology and merge in user-supplied configuration where safe.
      * **Injection:** We explicitly inject the `listener "tcp"` block pointing to the locations where we mounted the `CertManager` secrets (`/etc/bao/tls`).
  * **Discovery Strategy:**
      * We configure `retry_join` with `leader_api_addr` pointing to the predictable DNS names of the StatefulSet (`cluster-0.openbao-svc`, `cluster-1...`).

  * **Static Auto-Unseal Integration:**
      * On first reconcile, checks for the existence of `Secret/bao-unseal-key` (per-cluster name).
      * If missing, generates 32 cryptographically secure random bytes, base64-encodes them, and stores them as the unseal key in the Secret.
      * Mounts this Secret into the StatefulSet PodSpec at a fixed path (e.g., `/etc/bao/unseal/key`).
      * Injects a `seal "static"` stanza into `config.hcl`, for example:
        ```hcl
        seal "static" {
          current_key    = "file:///etc/bao/unseal/key"
          current_key_id = "operator-generated-v1"
        }
        ```
      * Ensures that the first leader initializes and auto-unseals OpenBao using this key, and that subsequent restarts remain unsealed automatically.

**Reconciliation Semantics**

- Watches `OpenBaoCluster`, `ConfigMap`, and `StatefulSet` resources.
- Ensures the rendered `config.hcl` and StatefulSet template are consistent with Spec (including multi-tenant naming conventions).
- Uses controller-runtime patch semantics to apply only necessary changes.
- Updates `Available`/`Degraded` conditions based on StatefulSet readiness.

### 4.2.1 Configuration Merge and Protected Stanzas

To avoid configuration conflicts:

- The Operator **owns and enforces** certain protected stanzas in `config.hcl`, including:
  - `listener "tcp"` (addresses, TLS paths, ports).
  - `storage "raft"` (path, `retry_join` blocks).
- User attempts to configure these protected sections directly in `spec.config` are rejected via validation.
- Non-protected settings (e.g., UI flags, telemetry, auth methods) MAY be provided via `spec.config` and are merged into the operator-generated template.
- The merge algorithm is deterministic and idempotent:
  - Start from an operator template containing protected stanzas.
  - Parse and merge user fragments, skipping or overriding only allowed keys.

### 4.3 The UpgradeManager (The State Machine)

**Responsibility:** Safe rolling updates.

**State Machine Logic:**

1.  **Detection:** `Spec.Version` \!= `Status.CurrentVersion`.
2.  **Lock:** Pause StatefulSet updates (Partition = Replicas).
3.  **Snapshot:** (Optional) Trigger a pre-upgrade snapshot.
4.  **Iterate (Reverse Ordinal):**
      * Target Pod $N$.
      * Is Pod $N$ the **Leader**?
          * **Yes:** Call `PUT /sys/step-down`. Wait for leadership change.
          * **No:** Proceed.
      * Decrement Partition (allow K8s to update Pod $N$).
      * Wait for `Ready` probe.
      * Wait for `sys/health` -\> `initialized: true`, `sealed: false`.
      * Wait for Raft Index to catch up (compare `last_log_index`).
5.  **Finalize:** Update `Status.CurrentVersion`.

**Reconciliation Semantics**

- Only one upgrade per `OpenBaoCluster` may run at a time (serialized via Status and Conditions).
- Transient OpenBao API errors cause retries with backoff; persistent failures result in `Upgrading=False` and `Degraded=True` conditions.
- Upgrades are designed to be resumable if the Operator restarts mid-upgrade (state is persisted in Status).

**Safety Checks and Split-Brain Handling**

- The UpgradeManager MUST verify that a healthy leader can be determined before proceeding:
  - If the Operator cannot determine a single, consistent leader (e.g., due to API timeouts, split-brain symptoms, or quorum loss), it MUST halt the upgrade.
  - In this case, it sets `Upgrading=False` and `Degraded=True` Conditions with a clear reason (e.g., `Reason=LeaderUnknown`), and leaves the StatefulSet partition unchanged.
- The UpgradeManager MUST NOT attempt step-down or pod updates when cluster health checks indicate loss of quorum or when multiple pods claim leadership.

-----

### 4.4 The BackupManager (Snapshots to Object Storage)

**Responsibility:** Stream Raft snapshots from the current leader to object storage.

- Watches `OpenBaoCluster` and backup-related configuration in Spec.
- On schedule:
  1. Discovers the current leader via the OpenBao API.
  2. Opens a snapshot stream via `GET /sys/storage/raft/snapshot`.
  3. Streams the body directly to object storage using an `io.Reader` (no buffering to disk).
  4. Updates `Status` with the last successful backup time and backup-related Conditions.

**Cloud-Agnostic Design**

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
  - Surface as typed `Conditions` (e.g., `ConditionDegraded`, `ConditionUpgrading`, `ConditionBackingUp`) with clear reasons and messages.
  - Avoid hot reconcile loops by relying on exponential backoff from the rate limiter.

-----

## 5\. Disaster Recovery Design

### 5.1 Backup Workflow

We utilize the `io.Pipe` pattern to stream data from OpenBao directly to object storage, keeping the Operator stateless and low-memory.

1.  **Scheduler:** Standard Kubernetes `CronJob` logic inside the Operator process.
2.  **Execution:**
      * Locate Leader (query K8s Endpoints or OpenBao API).
      * Open Stream: `GET /sys/storage/raft/snapshot`.
      * Open Upload Stream: object storage SDK `Upload()`.
      * Pipe Reader -\> Writer.
3.  **Encryption:** We rely on the object storage provider's server-side encryption or OpenBao's native snapshot encryption (if enterprise/configured), and we RECOMMEND:
      * Per-tenant or per-cluster buckets/prefixes for isolation.
      * Least-privilege credentials that can only write to the intended location.

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

**Security Context:**

  * Operator runs as non-root.
  * TLS Keys generated by the Operator never leave the cluster (stored in Secrets).

**Static Auto-Unseal Security Architecture:**

- The Operator uses OpenBao's static auto-unseal mechanism and manages a per-cluster unseal key stored in a Kubernetes Secret (e.g., `bao-unseal-key`).
- The Root of Trust for OpenBao data is therefore anchored in Kubernetes Secrets (and ultimately etcd), rather than an external cloud KMS.
- If an attacker can read Secrets in the OpenBao namespace, they can obtain the unseal key and decrypt OpenBao data.
- Mitigations and expectations:
  - Kubernetes etcd encryption at rest SHOULD be enabled and properly configured.
  - RBAC MUST strictly limit access to Secrets in namespaces where OpenBao clusters run.
  - Platform teams SHOULD apply additional controls such as node security hardening and audit logging for Secret access.
  - Rotation of the static auto-unseal key is **not automated** in v0.1; any key rotation procedure is a manual, carefully planned operation that requires re-encryption of data and is out of scope for this version.

Multi-tenancy implications:

- RBAC can be configured to allow namespace-scoped teams to manage only `OpenBaoCluster` resources in their own namespaces.
- The Operator implementation MUST avoid cross-namespace assumptions; all resource names must be fully qualified with namespace.
 - Backup destinations SHOULD be isolated per tenant or per cluster where possible, and backup credentials MUST follow least-privilege principles.

-----

## 7\. Observability & Metrics

The Operator exposes metrics suitable for Prometheus-style scraping. At a minimum:

- `openbao_cluster_ready_replicas` (gauge): number of Ready replicas per `OpenBaoCluster`.
- `openbao_upgrade_status` (gauge): numeric status of the last upgrade (e.g., 0=none, 1=running, 2=success, 3=failed).
- `openbao_backup_last_success_timestamp` (gauge): Unix timestamp of the last successful backup per cluster.
- `openbao_backup_failure_total` (counter): cumulative count of backup failures per cluster.
- `openbao_reconcile_duration_seconds` (histogram): reconciliation duration per controller and per cluster.
- `openbao_reconcile_errors_total` (counter): total reconciliation errors per controller, reason, and cluster.

Structured logs include `namespace`, `name` of the `OpenBaoCluster`, the controller name, and a correlation ID per reconciliation loop.

These metrics are used to validate and enforce the non-functional requirements:

- `openbao_reconcile_duration_seconds` underpins the target that, under normal load, reconciliations for an `OpenBaoCluster` complete within approximately 30 seconds.
- Per-cluster gauges and counters support running at least 10 concurrent `OpenBaoCluster` resources without unacceptable degradation in reconcile latency.
- Operator CPU and memory usage for typical cluster sizes MUST be measured during performance testing and documented for operators.

For clusters using the Prometheus Operator, we MAY ship example `ServiceMonitor`/`PodMonitor` resources that scrape the Operator's metrics endpoint; however, creation and wiring of these resources is environment-specific and remains the responsibility of platform teams.

-----

## 8\. Implementation Plan

### Phase 1: The Secure Foundation (Days 1-3)

  * **Goal:** `kubectl apply -f cluster.yaml` results in 3 running, mTLS-secured pods.
  * **Tasks:**
    1.  Project Scaffold (Kubebuilder).
    2.  Implement `CertManager` (CA & Cert generation).
    3.  Implement `ConfigMap` rendering (injecting the cert paths).
    4.  Implement basic `StatefulSet` reconciliation.

### Phase 2: Observability & Health (Days 4-5)

  * **Goal:** Operator knows who the Leader is and exposes basic metrics.
  * **Tasks:**
    1.  Implement OpenBao API Client in Go.
    2.  Update Status subresource with Leader info and Seal status.
    3.  Expose reconciliation metrics and structured logs for observability.

### Phase 3: The Upgrade Controller (Days 6-8)

  * **Goal:** Change version string and watch pods update one by one without downtime.
  * **Tasks:**
    1.  Implement Partition logic.
    2.  Implement Step-Down logic.

### Phase 4: Backups (Day 9+)

  * **Goal:** Scheduled object storage dumps.

-----

## 9\. Testing Strategy

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
