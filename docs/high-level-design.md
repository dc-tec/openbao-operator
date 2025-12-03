This is a **very strong** architectural pivot. Moving away from micromanaging the join process (which OpenBao/Vault handles natively via `retry_join`) and focusing on the "Supervisor" pattern for Day 2 operations is the correct path for a modern Kubernetes Operator.

The goal of this High-Level Design (HLD) is to describe the architecture, scope, key flows, and major design decisions of the **OpenBao Supervisor Operator** in a cloud-agnostic, multi-tenant Kubernetes environment.

-----

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

**Cluster Creation (Day 0):**
1. User creates an `OpenBaoCluster` CR in a namespace.
2. CertController bootstraps PKI (CA + leaf certs).
3. Infrastructure/ConfigController ensures a per-cluster static auto-unseal key Secret exists (creating it if missing).
4. ConfigController renders `config.hcl` with TLS, Raft storage, `retry_join`, and a `seal "static"` stanza that references the mounted unseal key.
5. StatefulSetController creates/updates the StatefulSet and associated Services, mounting TLS and unseal Secrets.
6. OpenBao pods start, automatically initialize and unseal using the static key, form a Raft cluster, and become Ready.

**Cluster Operations / Upgrades (Day 2):**
1. User updates `OpenBaoCluster.Spec.Image` or related configuration.
2. UpgradeController detects drift between Spec and Status and orchestrates Raft-aware rolling updates, respecting leader step-down semantics.
3. ConfigController and CertController maintain configuration and TLS independently from upgrade operations.

**Maintenance / Manual Recovery:**
1. User sets `OpenBaoCluster.Spec.Paused = true` to enter maintenance mode.
2. All reconcilers for that cluster short-circuit and stop mutating resources, allowing manual actions (e.g., manual restore from snapshot).
3. After maintenance, user sets `Paused = false` to resume normal reconciliation.

**Backups (Day N):**
1. User configures backup schedule and target object storage in the `OpenBaoCluster` spec.
2. BackupController periodically triggers snapshot streaming from the current leader to the configured object storage endpoint.
3. Backup status and last successful timestamp are recorded in `Status`.

**Deletion and Lifecycle:**
1. User deletes an `OpenBaoCluster` CR.
2. Finalizers determine whether to retain or delete PVCs and external backups according to a configurable deletion policy on the CR.
3. Operator removes Kubernetes resources once cleanup obligations are satisfied.

**External Access:**
1. User configures external access via `Service` (e.g., LoadBalancer) and/or `Ingress` fields in the `OpenBaoCluster` spec.
2. The Operator reconciles the corresponding Service/Ingress resources.
3. CertController ensures any external hostnames are reflected in TLS SANs (via `ExtraSANs` or derived from the spec).
4. Platform teams follow documented patterns for integrating with specific Ingress controllers and, where applicable, service meshes without breaking end-to-end mTLS.

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
          * `*.openbao.svc` (General internal comms)
          * `prod-cluster-0.openbao.svc` (Direct pod addressing)
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
      * If different: Execute `kill -SIGHUP 1` inside the OpenBao container. This forces OpenBao to reload the certs from disk immediately.

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
  
  # The Magic Sauce
  retry_join {
    leader_api_addr = "https://prod-cluster-0.openbao.svc:8200"
    leader_ca_cert_file = "/etc/bao/tls/ca.crt"
    # mTLS is critical here so pods trust each other
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file = "/etc/bao/tls/tls.key"
  }
  retry_join {
    leader_api_addr = "https://prod-cluster-1.openbao.svc:8200"
    # ... certs ...
  }
  # ... repeat for all replicas ...
}
```

*Note: Using explicit `retry_join` blocks for every predicted replica is often more reliable and faster than K8s auto-discovery tags in strict network policy environments.*

**Configuration Ownership and Merging**

- The Operator owns the critical `listener "tcp"` and `storage "raft"` stanzas to guarantee mTLS and Raft stability.
- Users may provide additional configuration fragments via `spec.config`, but attempts to override protected stanzas are rejected by validation.
- The ConfigController merges user fragments into an operator-generated template in a deterministic, idempotent way.

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
      * Hardcode a basic ConfigMap with `retry_join` pointing to the certs.
5.  **Test:** Apply CRD. Does the cluster form? Can you `kubectl exec` and see `openbao operator raft list-peers` showing 3 nodes?
