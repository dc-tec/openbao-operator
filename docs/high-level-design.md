# High-Level Design

### 1\. Scope, Context, and Non-Goals

- The Operator runs inside a Kubernetes cluster and manages one or more `OpenBaoCluster` custom resources across namespaces.
- Each `OpenBaoCluster` represents a logical OpenBao Raft cluster backed by a Kubernetes StatefulSet, persistent volumes, and object storage for backups.
- The Operator is **cloud-agnostic**: it assumes generic Kubernetes APIs and generic object storage (S3-/GCS-/Azure Blob-compatible) without relying on provider-specific features.
- Multi-tenancy is a first-class concern: multiple `OpenBaoCluster` instances can coexist per cluster and per namespace.

**Non-Goals (initial versions):**
- Cross-cluster or multi-region OpenBao federation.
- Automated disaster **restore** flows (restore may be manual for v0.1).
- Deep integration with provider-specific IAM mechanisms (e.g., IRSA); these can be layered on by platform teams.

-----

### 2\. Architecture Overview

At a high level, the architecture consists of:

- **Operator Controller Manager:** Runs multiple reconcilers (Cert, Config, StatefulSet, Upgrade, Backup) in a single process.
- **OpenBao StatefulSets:** One StatefulSet per `OpenBaoCluster` providing Raft storage and HA.
- **Persistent Volumes:** Provided by the cluster's StorageClass for durable Raft data.
- **Object Storage:** Generic HTTP/S-compatible object storage for snapshots (S3/GCS/Azure Blob or compatible endpoints).
- **Users / Tenants:** Developers and platform teams who create and manage `OpenBaoCluster` resources in their own namespaces.
- **Static Auto-Unseal Key:** A per-cluster unseal key managed by the Operator and stored in a Kubernetes Secret, used by OpenBao's static auto-unseal mechanism.

The Operator treats `OpenBaoCluster.Spec` as the declarative source of truth and continuously reconciles Kubernetes resources and OpenBao state to match this desired state.

-----

### 3\. Key Flows (Day 0, Day 2, Day N)

**Cluster Creation (Day 0) - Standard Initialization:**
1. User creates an `OpenBaoCluster` CR in a namespace (without `spec.selfInit`).
2. CertController bootstraps PKI (CA + leaf certs).
3. Infrastructure/ConfigController ensures a per-cluster static auto-unseal key Secret exists (creating it if missing).
4. ConfigController renders `config.hcl` with TLS, Raft storage, `retry_join`, and a `seal "static"` stanza that references the mounted unseal key.
5. StatefulSetController creates the StatefulSet with **1 replica initially** (regardless of `spec.replicas`), mounting TLS and unseal Secrets.
6. InitController waits for pod-0 to be running, then:
   - Checks if OpenBao is already initialized via the HTTP health endpoint (`GET /v1/sys/health`).
   - If not, calls the HTTP initialization endpoint (`PUT /v1/sys/init`) against pod-0 to initialize the cluster.
   - Stores the root token in a per-cluster Secret (`<cluster>-root-token`).
   - Sets `Status.Initialized = true`.
7. Once initialized, InfrastructureController scales the StatefulSet to the desired `spec.replicas`.
8. Additional OpenBao pods start, auto-unseal using the static key, join the Raft cluster via `retry_join`, and become Ready.

**Cluster Creation (Day 0) - Self-Initialization:**

When `spec.selfInit.enabled = true`, the cluster uses OpenBao's native self-initialization feature:

1. User creates an `OpenBaoCluster` CR with `spec.selfInit` configured.
2. CertController bootstraps PKI (CA + leaf certs).
3. Infrastructure/ConfigController ensures a per-cluster static auto-unseal key Secret exists.
4. ConfigController renders `config.hcl` with TLS, Raft storage, `retry_join`, `seal "static"`, **and `initialize` stanzas** based on `spec.selfInit.requests[]`.
5. StatefulSetController creates the StatefulSet with **1 replica initially**.
6. OpenBao automatically initializes itself on first start using the `initialize` stanzas:
   - Auto-unseals using the static key.
   - Executes all configured `initialize` requests (audit, auth, secrets, policies).
   - **The root token is NOT returned and is automatically revoked after use.**
7. InitController detects initialization via the HTTP health endpoint and sets `Status.Initialized = true`.
8. InfrastructureController scales the StatefulSet to the desired `spec.replicas`.
9. Additional OpenBao pods start, auto-unseal, and join the Raft cluster.

**Note:** Self-initialization requires an auto-unseal mechanism (which the Operator provides via static auto-unseal). No root token Secret is created when self-init is enabled.

**Note:** The static auto-unseal feature requires **OpenBao v2.4.0 or later**. Earlier versions do not support the `seal "static"` configuration.

**Cluster Operations / Upgrades (Day 2):**
1. User updates `OpenBaoCluster.Spec.Version` and/or `Spec.Image`.
2. UpgradeController detects version drift and performs pre-upgrade validation:
   - Validates semantic versioning (blocks downgrades by default).
   - Verifies all pods are Ready and quorum is healthy.
   - Optionally triggers a pre-upgrade backup if `spec.backup.preUpgradeSnapshot` is enabled.
3. UpgradeController orchestrates Raft-aware rolling updates:
   - Locks StatefulSet updates using partitioning.
   - Iterates pods in reverse ordinal order.
   - Forces leader step-down before updating leader pod.
   - Waits for pod Ready, OpenBao health, and Raft sync after each update.
4. Upgrade progress is persisted in `Status.Upgrade`, allowing resumption after Operator restart.
5. On completion, `Status.CurrentVersion` is updated and `Status.Upgrade` is cleared.

**Note:** Upgrades are designed to be safe and resumable. Downgrades are blocked by default. If an upgrade fails, it halts and sets `Degraded=True`; automated rollback is not supported.

**Maintenance / Manual Recovery:**
1. User sets `OpenBaoCluster.Spec.Paused = true` to enter maintenance mode.
2. All reconcilers for that cluster short-circuit and stop mutating resources, allowing manual actions (e.g., manual restore from snapshot).
3. If an upgrade was in progress, it is paused but state is preserved in `Status.Upgrade`.
4. After maintenance, user sets `Paused = false` to resume normal reconciliation (including any paused upgrade).

**Backups (Day N):**
1. User configures backup schedule (`spec.backup.schedule`) and target object storage in the `OpenBaoCluster` spec.
2. User configures authentication method:
   - **Kubernetes Auth (Preferred):** Set `spec.backup.kubernetesAuthRole` and configure the role in OpenBao
   - **Static Token (Fallback):** For all clusters, set `spec.backup.tokenSecretRef` pointing to a backup token Secret (root tokens are not used)
3. BackupController schedules backups using cron expressions (e.g., `"0 3 * * *"` for daily at 3 AM).
4. On schedule, BackupController:
   - Creates a Kubernetes Job with the backup executor container
   - Job uses `<cluster-name>-backup-serviceaccount` (automatically created by operator)
   - Backup executor:
     - Authenticates to OpenBao using Kubernetes Auth or static token
     - Discovers the current Raft leader via OpenBao API
     - Streams `GET /v1/sys/storage/raft/snapshot` directly to object storage (no disk buffering)
     - Names backups predictably: `<prefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap`
     - Verifies upload completion
5. Backup status is recorded in `Status.Backup`:
   - `LastBackupTime`, `NextScheduledBackup` for visibility
   - `ConsecutiveFailures` for alerting
6. Optional retention policies (`spec.backup.retention`) automatically delete old backups:
   - `MaxCount`: Keep only the N most recent backups
   - `MaxAge`: Delete backups older than a specified duration

**Note:** Backups are skipped during upgrades to avoid inconsistent snapshots. Backups are optional for all clusters. If backups are enabled, either `kubernetesAuthRole` or `tokenSecretRef` must be configured. Root tokens are not used for backup operations.

**Deletion and Lifecycle:**
1. User deletes an `OpenBaoCluster` CR.
2. Finalizers determine whether to retain or delete PVCs and external backups according to a configurable deletion policy on the CR.
3. Operator removes Kubernetes resources once cleanup obligations are satisfied.

**External Access:**
1. User configures external access via `Service` (e.g., LoadBalancer), `Ingress`, or `Gateway` fields in the `OpenBaoCluster` spec.
2. The Operator reconciles the corresponding Service/Ingress/HTTPRoute resources.
3. CertController ensures any external hostnames are reflected in TLS SANs (via `ExtraSANs` or derived from the spec).
4. Platform teams follow documented patterns for integrating with specific Ingress controllers, Gateway implementations, and service meshes without breaking end-to-end mTLS.

**Gateway API Support:**
1. User configures `spec.gateway` with a reference to an existing `Gateway` resource and routing configuration.
2. The Operator creates an `HTTPRoute` that routes traffic from the specified Gateway to the OpenBao Service.
3. Gateway hostnames are automatically added to the TLS SANs.
4. Gateway API provides a more expressive and portable alternative to Ingress, with support for advanced traffic management features.

-----

### 4\. Operator Lifecycle and Upgrade Strategies

The Operator supports two deployment/upgrade strategies, selectable by the platform team:

- **Externally Managed Operator:**
  - The Operator Deployment and CRDs are managed via Helm, GitOps, or another external system.
  - Upgrades are performed by updating the operator image/version in that external system.
  - The Operator does not attempt to self-upgrade.

- **Self-Managed Operator (Optional):**
  - The Operator may include an optional `OperatorConfig` or similar CRD to describe its own desired version and configuration.
  - A dedicated controller can reconcile the operator Deployment, allowing "in-cluster" self-management.

In both scenarios:
- Backwards-compatible CRD changes are preferred.
- When breaking API changes are needed, new CRD versions (e.g., `v1alpha1` → `v1beta1`) and conversion webhooks will be introduced, with clear migration paths.

-----

### 5\. The Core API: Deep Dive into `OpenBaoCluster`

The HLD CRD is a good start, but for the implementation (Go structs), we need to define the **Status** subresource rigorously. The Operator makes decisions based on the delta between `Spec` and `Status`.

#### Revised CRD Specification (Go Struct Draft)

```go
type OpenBaoClusterSpec struct {
    Version  string `json:"version"` // e.g., "2.1.0"
    Replicas int32  `json:"replicas"`

    // If true, reconciliation is paused for this cluster (maintenance mode).
    Paused   bool   `json:"paused,omitempty"`
    
    // TLS definitions as per HLD
    TLS TLSConfig `json:"tls"`
    
    // Using a map allows flexible configuration of the native OpenBao config
    // The Operator will merge this with its required overrides (listener, storage)
    Config map[string]string `json:"config,omitempty"` // user-supplied fragments (non-protected keys only)

    // SelfInit configures OpenBao's native self-initialization feature.
    // When enabled, OpenBao initializes itself on first start using the
    // configured requests, and the root token is automatically revoked.
    SelfInit *SelfInitConfig `json:"selfInit,omitempty"`

    // Gateway configures Kubernetes Gateway API access (alternative to Ingress).
    Gateway *GatewayConfig `json:"gateway,omitempty"`
}

type SelfInitConfig struct {
    // Enabled activates OpenBao's self-initialization feature.
    // When true, the Operator injects `initialize` stanzas into config.hcl
    // and does NOT create a root token Secret (root token is auto-revoked).
    Enabled bool `json:"enabled"`
    
    // Requests defines the API operations to execute during self-initialization.
    // Each request becomes an `initialize` stanza in the OpenBao config.
    Requests []SelfInitRequest `json:"requests,omitempty"`
}

type SelfInitRequest struct {
    // Name is a unique identifier for this request (used as the stanza name).
    // Must match regex ^[A-Za-z_][A-Za-z0-9_-]*$
    Name string `json:"name"`
    
    // Operation is the API operation type (e.g., "update", "create", "delete").
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
    AllowFailure bool `json:"allowFailure,omitempty"`
}

type GatewayConfig struct {
    // Enabled activates Gateway API support for this cluster.
    Enabled bool `json:"enabled"`
    
    // GatewayRef references an existing Gateway resource.
    GatewayRef GatewayReference `json:"gatewayRef"`
    
    // Hostname for routing traffic to this OpenBao cluster.
    Hostname string `json:"hostname"`
    
    // Path prefix for the HTTPRoute (defaults to "/").
    Path string `json:"path,omitempty"`
    
    // Annotations to apply to the HTTPRoute.
    Annotations map[string]string `json:"annotations,omitempty"`
}

type GatewayReference struct {
    // Name of the Gateway resource.
    Name string `json:"name"`
    
    // Namespace of the Gateway resource. If empty, uses the OpenBaoCluster namespace.
    Namespace string `json:"namespace,omitempty"`
}

type OpenBaoClusterStatus struct {
    // The Phase is the high-level summary for the user
    // "Initializing", "Running", "Upgrading", "BackingUp", "Failed"
    Phase string `json:"phase"`

    // Critical for the Upgrade Controller to know when to proceed
    ActiveLeader string `json:"activeLeader,omitempty"` // e.g., "prod-cluster-0"
    
    // Critical for ensuring the StatefulSet is stable
    ReadyReplicas int32 `json:"readyReplicas"`
    
    // Tracks the current version running on the cluster (vs Spec.Version)
    CurrentVersion string `json:"currentVersion"`

    // Indicates whether the cluster has been initialized via bao operator init
    // or self-initialization. Used by InfraManager to determine when to scale
    // beyond 1 replica.
    Initialized bool `json:"initialized,omitempty"`

    // SelfInitialized indicates whether the cluster was initialized using
    // OpenBao's self-initialization feature. When true, no root token Secret
    // exists for this cluster.
    SelfInitialized bool `json:"selfInitialized,omitempty"`
    
    // Last successful backup timestamp
    LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
}
```

-----

### 6\. Module 1: Automated TLS Authority (The Implementation Details)

You identified this as the "Killer Feature." To implement this securely, we must handle the **Certificate Authority (CA)** and **Subject Alternative Names (SANs)** correctly so `retry_join` works.

#### A. The Internal CA Logic

The Operator will use Go's standard `crypto/x509` library.

1.  **Bootstrapping:**
      * Check for Secret `prod-cluster-ca`.
      * If missing: Generate a 2048-bit RSA key and a Self-Signed Root CA Certificate (valid for 10 years). Save to Secret.
2.  **Leaf Issuance:**
      * Generate a leaf certificate signed by the stored CA.
      * **Crucial Step - SANs:** You must inject specific DNS entries for K8s service discovery to work during `retry_join`.
          * `*.security.svc` (General internal comms within the namespace)
          * `prod-cluster.security.svc` (Service-level addressing)
          * `*.prod-cluster.security.svc` (Direct pod addressing via the headless Service)
          * `127.0.0.1` (Localhost access)
3.  **Mounting:**
      * The Secret `bao-tls` is mounted at `/etc/bao/tls`.

#### B. The Rotation Loop (Reconciliation)

The HLD mentions `SIGHUP`. Here is the specific technical implementation for the Reconciler:

1.  **Watch:** The Controller watches the `bao-tls` Secret.
2.  **Check:** In the loop, parse the `tls.crt`. Is `NotAfter - Now < 7 days`?
3.  **Renew:** If yes, regenerate certs and update the Secret.
4.  **Signal:**
      * Kubernetes updates the mounted volume automatically (eventually).
      * **Optimization:** The Operator watches the Pod's *file system*? No, the Operator can't see inside.
      * **Better Approach:** The Operator compares the `ResourceVersion` of the Secret it just updated against an annotation on the Pod.
      * If different: Update the `openbao.org/tls-cert-hash` annotation on each ready OpenBao pod. A small sidecar container running in the same pod observes this change (or the underlying TLS volume updates) and sends `SIGHUP` to the OpenBao process locally, forcing it to reload the certs from disk without requiring `pods/exec` from the operator.

-----

### 7\. Module 2: Raft-Aware Upgrades (The State Machine)

This is the most complex logic. We will use **StatefulSet Partitioning**. This allows the Operator to control exactly which Pods are updated by Kubernetes, rather than letting Kubernetes update them all at once.

**The Algorithm:**

1.  **Trigger:** User changes `Spec.Version` to `2.1.0`. `Status.CurrentVersion` is `2.0.0`.
2.  **Set Strategy:** Operator patches StatefulSet `updateStrategy` to `RollingUpdate` with `partition: <replicas>`. (This pauses K8s updates).
3.  **Identify Leader:** Operator calls `GET /v1/sys/health` on all pods.
      * *Result:* Pod-0 (Follower), Pod-1 (Leader), Pod-2 (Follower).
4.  **Update Followers (Reverse Ordinal):**
      * Operator decrements partition to `2`. Pod-2 updates.
      * Operator waits for Pod-2 `Ready` check + OpenBao Health check.
      * Operator decrements partition to `1`. Pod-0 updates (Skipping Pod-1 because it's Leader).
      * *Correction:* StatefulSets process by index (2-\>1-\>0). If Pod-1 is Leader, we face a problem. Kubernetes *insists* on updating 2, then 1, then 0.
      * **Revised Logic (The Step-Down):** We cannot skip indices in a StatefulSet. Therefore, **we must force a step-down BEFORE the update reaches the leader's index.**

**Revised Upgrade Flow:**

1.  Target: Pod-2. Update it. Wait for health.
2.  Target: Pod-1. Is it Leader? **Yes.**
3.  **Action:** Send `PUT /v1/sys/step-down` to Pod-1. Wait for leadership to transfer to Pod-0 or Pod-2.
4.  Once Pod-1 is a follower, decrement partition to allow K8s to update Pod-1.
5.  Repeat for Pod-0.

-----

### 8\. Module 3: Disaster Recovery (Backup Implementation)

Using the K8s API to proxy the Snapshot request is cleaner than a sidecar.

**Technical Requirements:**

1.  **Credentials:** The Operator needs write access to the configured object storage endpoint.
      * The mechanism for obtaining credentials is deployment/environment-specific (e.g., Kubernetes Secrets, workload identity, IAM Roles for Service Accounts) and is left to platform teams.
2.  **Streaming:**
      * Do *not* download the snapshot to the Operator's memory/disk first (OOM risk).
      * Use `io.Pipe`. The HTTP Body from OpenBao (`GET /sys/storage/raft/snapshot`) should be piped directly to the object storage SDK stream.

<!-- end list -->

```go
// Psuedo-code for streaming snapshot
func (r *BackupReconciler) performBackup(ctx context.Context, leaderURL string) error {
    // 1. Open Stream from OpenBao
    resp, _ := http.Get(leaderURL + "/v1/sys/storage/raft/snapshot")
    defer resp.Body.Close()

    // 2. Stream to S3 (Pipe)
    // uploader := <object storage uploader>
    // _, err := uploader.Upload(&UploadInput{
    //     Bucket: "my-backups",
    //     Key:    "backup-" + time.Now().String() + ".snap",
    //     Body:   resp.Body, // Stream directly
    // })
    return err
}
```

-----

### 9\. Day 0 Bootstrapping (The ConfigMap Injection)

You mentioned relying on `retry_join`. The Operator must generate the OpenBao configuration file dynamically.

**The `config.hcl` template generated by the Operator:**

```hcl
ui = true
disable_mlock = true

listener "tcp" {
  address = "[::]:8200"
  cluster_address = "[::]:8201"
  # Operator injects paths to the Secret it manages
  tls_cert_file = "/etc/bao/tls/tls.crt"
  tls_key_file  = "/etc/bao/tls/tls.key"
  tls_client_ca_file = "/etc/bao/tls/ca.crt"
}

seal "static" {
  current_key    = "file:///etc/bao/unseal/key"
  current_key_id = "operator-generated-v1"
}

storage "raft" {
  path = "/bao/data"

  # The Magic Sauce (bootstrap)
  # During initial bootstrap, only pod-0 is guaranteed to exist.
  # The Operator renders a deterministic retry_join targeting pod-0
  # so that the first node can form a Raft cluster.
  retry_join {
    leader_api_addr = "https://prod-cluster-0.prod-cluster.security.svc:8200"
    leader_ca_cert_file = "/etc/bao/tls/ca.crt"
    # mTLS is critical here so pods trust each other
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file = "/etc/bao/tls/tls.key"
  }

  # After initialization, the Operator switches to a dynamic
  # Kubernetes go-discover based retry_join so that new pods
  # can join by label instead of a static peer list.
  retry_join {
    auto_join               = "provider=k8s namespace=security label_selector=\"openbao.org/cluster=prod-cluster\""
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
```

*Note: We use a hybrid approach. A deterministic bootstrap `retry_join` to pod-0 avoids split-brain during Day 0, while the post-initialization `auto_join` with the Kubernetes provider (`provider=k8s`) enables dynamic, label-driven scaling without editing the ConfigMap.*

**Configuration Ownership and Merging**

- The Operator owns the critical `listener "tcp"` and `storage "raft"` stanzas to guarantee mTLS and Raft stability.
- Users may provide additional non-protected configuration settings via `spec.config` as key/value pairs, but attempts to override protected stanzas are rejected by validation enforced at admission time:
  - CRD-level CEL rules block obvious misuse (for example, top-level `listener`, `storage`, `seal` keys).
  - A validating admission webhook performs deeper, semantic checks and enforces additional constraints on `spec.config` keys and values.
- The ConfigController merges user attributes into an operator-generated template in a deterministic, idempotent way, skipping any operator-owned keys.

-----

### 10\. Development Roadmap (Refined)

Since TLS is the foundation, here is the immediate execution order:

1.  **Scaffold:** `kubebuilder init --domain openbao.org --repo github.com/openbao/operator`
2.  **API:** Define `OpenBaoCluster` struct (focus on Spec.TLS).
3.  **Controller (Cert Manager):**
      * Implement `CreateCA` (generates Secret).
      * Implement `CreateCert` (generates Secret signed by CA).
4.  **Controller (Infra):**
      * Implement `ReconcileStatefulSet`.
      * Hardcode a basic ConfigMap with bootstrap `retry_join` pointing to pod-0 and TLS certs, then extend it with a Kubernetes go-discover based `auto_join` for post-initialization scaling.
5.  **Test:** Apply CRD. Does the cluster form? Can you `kubectl exec` and see `openbao operator raft list-peers` showing 3 nodes?
