# Threat Model: OpenBao Operator

## 1. Asset Identification
* **Critical:** Root token stored in Kubernetes Secret (`<cluster>-root-token`) - grants full administrative access to OpenBao.
* **High Value:** Root CA Private Key (stored in Secret).
* **High Value:** Static auto-unseal key stored in Kubernetes Secret (`<cluster>-unseal-key`) per OpenBao cluster.
* **High Value:** OpenBao Raft data on persistent volumes (PVCs).
* **High Value:** Backup snapshots stored in external object storage.
* **Medium Value:** The OpenBao Configuration (ConfigMap).
* **Medium Value:** Operator service account tokens, backup ServiceAccount tokens, and any credentials/secrets used to access object storage.
* **Medium Value:** `OpenBaoCluster` CRDs and their Status (can reveal topology and health).

## 2. Threat Analysis (STRIDE)

### A. Spoofing
* **Threat:** A rogue pod attempts to join the Raft cluster.
* **Mitigation:** We use strict mTLS. The Operator acts as the CA. Only pods with a valid certificate mounted via the Secret can join the mesh.

* **Threat:** An attacker spoofs external endpoints (Ingress/LoadBalancer) to intercept OpenBao traffic.
* **Mitigation:** Require TLS with correct SANs for all external hostnames; the Operator automatically creates NetworkPolicies to enforce cluster isolation (default deny all ingress, allow same-cluster and kube-system traffic). Additionally, recommend service mesh mTLS for north-south traffic where applicable.

### B. Tampering
* **Threat:** User manually edits the StatefulSet (e.g., changes image tag).
* **Mitigation:** The Operator's Reconciliation loop runs constantly. It will revert any out-of-band changes to the StatefulSet to match the CRD "Source of Truth" immediately.

* **Threat:** Malicious or misconfigured tenant modifies their `OpenBaoCluster` to point backups to an unauthorized object storage location.
* **Mitigation:** Backup target configuration can be constrained via admission control policies and/or namespace-scoped RBAC; operators should validate backup targets against an allowlist where required.

### C. Elevation of Privilege
* **Threat:** Attacker compromises the Operator Pod to gain control of the cluster.
* **Mitigation:**
    * Operator runs as non-root.
    * RBAC `ClusterRole` is scoped strictly to `openbao.org` resources, Secrets, and StatefulSets.
    * Operator does NOT have permission to read the *data* inside OpenBao (KV engine), only the API system endpoints.

* **Threat:** One tenant's `OpenBaoCluster` configuration impacts other clusters (cross-tenant impact).
* **Mitigation:**
    * All resources use deterministic naming: `<cluster-name>-<suffix>` pattern.
    * Resources are always created in the `OpenBaoCluster` namespace.
    * Resources are labeled with `openbao.org/cluster=<cluster-name>` for proper identification.
    * RBAC supports namespace-scoped user access via RoleBindings.
    * Controller rate limiting (MaxConcurrentReconciles: 3, exponential backoff) prevents one cluster from starving others.
    * Example namespace-scoped RoleBindings provided in `config/samples/namespace_scoped.yaml`.

### D. Information Disclosure
* **Threat:** TLS keys, backup credentials, or snapshot contents are exposed via logs or metrics.
* **Mitigation:** Never log secret material; use structured logging with careful redaction; encrypt Secrets at rest and enforce least-privilege RBAC; rely on OpenBao's telemetry parameters (not custom logging of sensitive fields).

* **Threat:** Unauthorized access to backups in object storage.
* **Mitigation:** Use storage-level encryption and strict access controls (per-tenant buckets/prefixes, scoped credentials); document best practices for platform teams.

* **Threat:** Backup credentials Secret (`spec.backup.target.credentialsSecretRef`) or backup ServiceAccount is compromised, allowing unauthorized access to backup storage.
* **Mitigation:**
    * Use least-privilege credentials that only allow write access to the specific bucket/prefix for the cluster.
    * Prefer workload identity (IRSA, GKE Workload Identity) over static credentials where possible.
    * Enable audit logging for access to credentials Secrets.
    * Rotate credentials periodically.

* **Threat:** Snapshot data contains sensitive OpenBao secrets and could be exfiltrated via backups.
* **Mitigation:**
    * Backups are encrypted at rest by the object storage provider (SSE-S3, GCS encryption, etc.).
    * Per-tenant bucket/prefix isolation prevents cross-tenant access.
    * Backup credentials should be scoped to prevent read access (write-only for backups, read for restore is a separate credential).
    * Document that restores should be performed in secure environments.

* **Threat (Critical):** Compromise of Kubernetes Secrets storing the static auto-unseal key allows decryption of all OpenBao data for that cluster.
* **Mitigation:** Require etcd encryption at rest; strictly limit Secret access via RBAC; apply network and node hardening; enable audit logging for Secret access in the Kubernetes control plane.

* **Threat (Critical):** Compromise of the root token Secret (`<cluster>-root-token`) grants full administrative access to the OpenBao cluster.
* **Mitigation:**
    * RBAC MUST strictly limit access to the root token Secret. Only cluster administrators and the Operator ServiceAccount should have read access.
    * Enable audit logging for all access to this Secret.
    * Organizations SHOULD revoke or rotate the root token after initial cluster setup and use more granular policies for ongoing administration.
    * The Operator intentionally does NOT log the root token or the initialization response.
    * Platform teams SHOULD periodically audit who has access to these Secrets.

### E. Denial of Service
* **Threat:** Misconfigured `OpenBaoCluster` resources cause hot reconcile loops that overload the Operator or API server.
* **Mitigation:** Implement reconciliation backoff, limit concurrent reconciles per cluster, and surface misconfiguration via Status conditions so users can remediate.

* **Threat:** Excessive backup frequency or large snapshot sizes exhaust object storage quotas or saturate network bandwidth.
* **Mitigation:** Validate backup schedules and document recommended limits; make backup settings configurable and observable.

### F. Repudiation
* **Threat:** Lack of audit trail for Operator-initiated actions (e.g., upgrades, step-downs, backups).
* **Mitigation:** 
    * The Operator emits structured audit logs for critical operations (initialization, leader step-down) tagged with `audit=true` and `event_type` for easy filtering in log aggregation systems.
    * Use structured, timestamped logs including `OpenBaoCluster` namespace/name and correlation IDs.
    * Rely on Kubernetes/audit logs and OpenBao audit logs (configurable via `spec.audit`) where available.

## 3. Secrets Management
* **Requirement:** The CA Key generated by the Operator must never be logged to stdout/stderr.
* **Requirement:** The root token and initialization response (containing unseal keys) must NEVER be logged.
* **Requirement:** Credentials and tokens used for object storage access must be stored in Kubernetes Secrets or via workload identity, never embedded in ConfigMaps or images.
* **Requirement:** Secret names must be unique per `OpenBaoCluster` to prevent accidental cross-tenant sharing.
* **Requirement:** The root token Secret (`<cluster>-root-token`) must be protected with strict RBAC; access should be limited to cluster administrators and the Operator.
* **Recommendation:** Organizations should revoke or rotate the root token after initial cluster setup, storing it securely offline for emergency recovery only.

## 4. Backup Authentication Security

The BackupManager requires authenticated access to OpenBao's snapshot API (`GET /v1/sys/storage/raft/snapshot`).

### 4.1 Authentication Methods

The operator supports two authentication methods for backup operations, in order of preference:

| Method | Cluster Type | Security Considerations |
|--------|--------------|-------------------------|
| **Kubernetes Auth (Preferred)** | All clusters | ServiceAccount tokens automatically rotated by Kubernetes. Limited lifetime. Better security. |
| **Static Token (Fallback)** | All clusters | User-configured backup token via `spec.backup.tokenSecretRef`. Must be explicitly configured with minimal permissions. |

### 4.2 Recommended Backup Policy

For production environments, configure a dedicated backup policy instead of using the root token:

```hcl
# Minimal policy for backup operations
path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}
```

### 4.3 Kubernetes Auth Configuration (Preferred)

For all clusters, Kubernetes Auth is the recommended authentication method:

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
      # The ServiceAccount name is automatically <cluster-name>-backup-serviceaccount
      - name: create-backup-kubernetes-role
        operation: update
        path: auth/kubernetes/role/backup
        data:
          bound_service_account_names: <cluster-name>-backup-serviceaccount
          bound_service_account_namespaces: <cluster-namespace>
          policies: backup
          ttl: 1h
  backup:
    kubernetesAuthRole: backup
```

**Security Benefits:**
- ServiceAccount tokens are automatically rotated by Kubernetes
- Tokens have limited lifetime (configured via `ttl` in the role)
- No long-lived credentials stored in Secrets
- Better audit trail (tokens tied to ServiceAccount identity)

### 4.4 Static Token Configuration (Fallback)

For self-init clusters using static tokens, users MUST configure a backup-capable token. Example:

```yaml
spec:
  selfInit:
    enabled: true
    requests:
      # Create backup policy
      - name: create-backup-policy
        operation: update
        path: sys/policies/acl/backup
        data:
          policy: |
            path "sys/storage/raft/snapshot" { capabilities = ["read"] }
      # Create AppRole for backups
      - name: enable-approle
        operation: update
        path: sys/auth/approle
        data:
          type: approle
      - name: create-backup-role
        operation: update
        path: auth/approle/role/operator-backup
        data:
          token_policies: ["backup"]
```

### 4.5 Backup Token Exposure

* **Threat:** Backup token is exposed, allowing unauthorized snapshot access.
* **Mitigation:**
    * Backup tokens should have minimal permissions (read `sys/storage/raft/snapshot` only).
    * Token TTL should be short; the Operator should handle token renewal.
    * Audit logging should be enabled to detect unauthorized snapshot access.

## 5. Upgrade Security Considerations

### 5.1 API Authentication During Upgrades

The UpgradeManager requires authenticated access to OpenBao APIs:
- `GET /v1/sys/health` - health and leader detection
- `PUT /v1/sys/step-down` - leader step-down

The Operator uses the root token for these operations. For self-init clusters, users must ensure appropriate permissions are configured.

### 5.2 Upgrade-Related Threats

* **Threat:** Malicious image is specified in `spec.image`, deploying compromised OpenBao binary.
* **Mitigation:**
    * Organizations SHOULD maintain an allowlist of approved OpenBao images.
    * Use image signing and verification (e.g., cosign, Notary).
    * Admission controllers can enforce image policies.

* **Threat:** Upgrade to vulnerable OpenBao version introduces security vulnerabilities.
* **Mitigation:**
    * Downgrades are blocked by default to prevent rollback to known-vulnerable versions.
    * Organizations SHOULD subscribe to OpenBao security advisories.
    * Document and enforce minimum supported version.

* **Threat:** Partial upgrade leaves cluster in inconsistent security state.
* **Mitigation:**
    * Upgrade state is persisted; Operator will resume and complete upgrade after restart.
    * Mixed-version clusters are surfaced via `Status.Conditions` for visibility.
    * Manual intervention guidance is documented.

## 6. Multi-Tenancy Security Considerations

In multi-tenant deployments, additional threats and mitigations apply:

### 6.1 Tenant Isolation Threats

* **Threat:** Tenant A reads Tenant B's root token or unseal key Secret.
* **Mitigation:**
    * Use namespace-scoped RoleBindings that do NOT grant Secret read access.
    * Deploy policy engines (OPA Gatekeeper, Kyverno) to block access to `*-root-token` and `*-unseal-key` Secrets.
    * Consider using self-initialization to avoid root token Secrets entirely.

* **Threat:** Tenant A's OpenBao pods communicate with Tenant B's pods.
* **Mitigation:**
    * The Operator automatically creates NetworkPolicies for each cluster that restrict ingress/egress to pods with matching `openbao.org/cluster` labels, enforcing default-deny-all-ingress with exceptions for:
      - Same-cluster pods (for Raft communication)
      - kube-system namespace (for DNS and system components)
      - OpenBao operator pods (for health checks, initialization, upgrades)
      - Egress to Kubernetes API server (for pod discovery via discover-k8s provider)

* **Threat:** Tenant A accesses Tenant B's backup data in object storage.
* **Mitigation:**
    * Each tenant MUST have separate backup credentials.
    * IAM policies MUST restrict access to tenant-specific bucket prefixes.
    * Backup credentials should be write-only (read access granted separately for restore operations).

### 6.2 Resource Exhaustion Threats

* **Threat:** One tenant creates excessive OpenBaoCluster resources, exhausting cluster resources.
* **Mitigation:**
    * Configure ResourceQuotas per namespace to limit pods, PVCs, CPU, and memory.
    * Controller rate limiting (MaxConcurrentReconciles: 3) prevents one cluster from starving the reconciler.

### 6.3 Recommended Multi-Tenancy Controls

For secure multi-tenant deployments, platform teams SHOULD implement:

| Control | Purpose | Implementation |
|---------|---------|----------------|
| Namespace-scoped RBAC | Isolate tenant access to CRs | RoleBindings with `openbaocluster-editor-role` |
| Secret access restrictions | Protect root tokens and unseal keys | Policy engine rules or explicit RBAC denies |
| NetworkPolicies | Isolate cluster network traffic | Automatically created by Operator (default deny all ingress, allow same-cluster and kube-system) |
| ResourceQuotas | Prevent resource exhaustion | Namespace-level quotas |
| Backup credential isolation | Protect backup data | Per-tenant IAM credentials with prefix restrictions |
| Pod Security Standards | Enforce secure pod configuration | PSA labels on namespaces |
| Audit logging | Monitor sensitive access | Kubernetes audit policy for Secrets |

See `docs/usage-guide.md` Section 10 for detailed implementation examples.
