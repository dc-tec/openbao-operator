# Threat Model

This section provides a comprehensive threat analysis using the STRIDE framework.

## 1. Asset Identification

- **Critical:** Root token stored in Kubernetes Secret (`<cluster>-root-token`) - grants full administrative access to OpenBao.
- **High Value:** Root CA Private Key (stored in Secret).
- **High Value:** Static auto-unseal key stored in Kubernetes Secret (`<cluster>-unseal-key`) per OpenBao cluster.
- **High Value:** OpenBao Raft data on persistent volumes (PVCs).
- **High Value:** Backup snapshots stored in external object storage.
- **Medium Value:** The OpenBao Configuration (ConfigMap).
- **Medium Value:** Operator service account tokens, backup ServiceAccount tokens, and any credentials/secrets used to access object storage.
- **Medium Value:** `OpenBaoCluster` CRDs and their Status (can reveal topology and health).

## 2. Threat Analysis (STRIDE)

### A. Spoofing

**Threat:** A rogue pod attempts to join the Raft cluster.

**Mitigation:** We use strict mTLS. The Operator acts as the CA. Only pods with a valid certificate mounted via the Secret can join the mesh.

**Threat:** An attacker spoofs external endpoints (Ingress/LoadBalancer) to intercept OpenBao traffic.

**Mitigation:** Require TLS with correct SANs for all external hostnames; the Operator automatically creates NetworkPolicies to enforce cluster isolation (default deny all ingress, allow same-cluster and kube-system traffic). Additionally, recommend service mesh mTLS for north-south traffic where applicable.

### B. Tampering

**Threat:** User manually edits the StatefulSet (e.g., changes image tag).

**Mitigation:** The Operator's Reconciliation loop runs constantly. It will revert any out-of-band changes to the StatefulSet to match the CRD "Source of Truth" immediately.

**Threat:** Malicious or misconfigured tenant modifies their `OpenBaoCluster` to point backups to an unauthorized object storage location.

**Mitigation:** Backup target configuration can be constrained via admission control policies and/or namespace-scoped RBAC; operators should validate backup targets against an allowlist where required.

### C. Elevation of Privilege

**Threat:** Attacker compromises the Operator Pod to gain control of the cluster.

**Mitigation:**

- Operator runs as non-root.
- **Least-Privilege RBAC Model:** The Operator uses separate ServiceAccounts:
  - **Provisioner ServiceAccount:** Has minimal cluster-wide permissions (namespace watching + RBAC management). Cannot access workload resources directly.
  - **OpenBaoCluster Controller ServiceAccount:** Has cluster-wide read access to `openbaoclusters` (`get;list;watch`) to support controller-runtime watches/caching, and cluster-wide `create` on `tokenreviews`/`subjectaccessreviews` for the protected metrics endpoint; all OpenBaoCluster writes and all child resources remain tenant-scoped via namespace Roles.
- **Namespace Isolation:** The OpenBaoCluster controller cannot access resources outside tenant namespaces or non-OpenBao resources within tenant namespaces.
- Operator does NOT have permission to read the *data* inside OpenBao (KV engine), only the API system endpoints.
- **Blind Writes:** For sensitive assets like the Unseal Key Secret, the operator uses a "blind create" pattern. It attempts to create the Secret but does not require `GET` or `LIST` permissions on the generated secret data after creation, minimizing the attack surface if the operator is compromised.

**Threat:** One tenant's `OpenBaoCluster` configuration impacts other clusters (cross-tenant impact).

**Mitigation:**

- All resources use deterministic naming: `<cluster-name>-<suffix>` pattern.
- Resources are always created in the `OpenBaoCluster` namespace.
- Resources are labeled with `openbao.org/cluster=<cluster-name>` for proper identification.
- **Namespace-Scoped RBAC:** The OpenBaoCluster controller receives permissions only via namespace-scoped Roles, preventing cross-namespace access.
- RBAC supports namespace-scoped user access via RoleBindings.
- Controller rate limiting (MaxConcurrentReconciles: 3, exponential backoff) prevents one cluster from starving others.

**Threat:** Compromised Provisioner controller accesses workload resources in tenant namespaces (e.g., reading other applications' Secrets).

**Mitigation:**

- The Provisioner ServiceAccount has cluster-wide permissions only for granting permissions (required by Kubernetes RBAC), but the Provisioner controller code does NOT use these permissions to access workload resources.
- The Provisioner only creates Roles/RoleBindings; it does not perform GET/LIST operations on StatefulSets, Secrets, Services, etc.
- The OpenBaoCluster controller (which does access workload resources) uses a separate ServiceAccount with NO cluster-wide permissions, only namespace-scoped permissions in tenant namespaces.
- This separation ensures that even if the Provisioner is compromised, it cannot read or modify workload resources.

**Threat:** Compromised Provisioner enumerates namespaces to discover cluster topology and project names.

**Mitigation:**

- The Provisioner no longer has `list` or `watch` permissions on namespaces. It can only `get` namespaces that are explicitly declared in `OpenBaoTenant` CRDs.
- The Provisioner watches `OpenBaoTenant` CRDs instead of namespaces, making it a "blind executor" that only acts on explicit governance instructions.
- Creating an `OpenBaoTenant` CRD requires write access to the operator's namespace, which should be highly restricted (Cluster Admin only).
- This eliminates the ability for a compromised Provisioner to survey the cluster topology.

### D. Information Disclosure

**Threat:** TLS keys, backup credentials, or snapshot contents are exposed via logs or metrics.

**Mitigation:** Never log secret material; use structured logging with careful redaction; encrypt Secrets at rest and enforce least-privilege RBAC; rely on OpenBao's telemetry parameters (not custom logging of sensitive fields).

**Threat:** Unauthorized access to backups in object storage.

**Mitigation:** Use storage-level encryption and strict access controls (per-tenant buckets/prefixes, scoped credentials); document best practices for platform teams.

**Threat:** Backup credentials Secret (`spec.backup.target.credentialsSecretRef`) or backup ServiceAccount is compromised, allowing unauthorized access to backup storage.

**Mitigation:**

- Use least-privilege credentials that only allow write access to the specific bucket/prefix for the cluster.
- Prefer workload identity over static credentials where possible:
  - Configure `spec.backup.target.roleArn` and omit `credentialsSecretRef` to use Web Identity (OIDC federation).
  - The backup Job mounts a projected ServiceAccount token and relies on the cloud SDK default credential chain.
- Enable audit logging for access to credentials Secrets.
- Rotate credentials periodically.

**Threat:** Snapshot data contains sensitive OpenBao secrets and could be exfiltrated via backups.

**Mitigation:**

- Backups are encrypted at rest by the object storage provider (SSE-S3, GCS encryption, etc.).
- Per-tenant bucket/prefix isolation prevents cross-tenant access.
- Backup credentials should be scoped to prevent read access (write-only for backups, read for restore is a separate credential).
- Document that restores should be performed in secure environments.

**Threat (Critical):** Compromise of Kubernetes Secrets storing the static auto-unseal key allows decryption of all OpenBao data for that cluster.

**Mitigation:** Require etcd encryption at rest; strictly limit Secret access via RBAC; apply network and node hardening; enable audit logging for Secret access in the Kubernetes control plane.

**Threat (Critical):** Compromise of the root token Secret (`<cluster>-root-token`) grants full administrative access to the OpenBao cluster.

**Mitigation:**

- RBAC MUST strictly limit access to the root token Secret. Only cluster administrators and the Operator ServiceAccount should have read access.
- Enable audit logging for all access to this Secret.
- Organizations SHOULD revoke or rotate the root token after initial cluster setup and use more granular policies for ongoing administration.
- The Operator intentionally does NOT log the root token or the initialization response.
- Platform teams SHOULD periodically audit who has access to these Secrets.

### E. Denial of Service

**Threat:** Misconfigured `OpenBaoCluster` resources cause hot reconcile loops that overload the Operator or API server.

**Mitigation:** Implement reconciliation backoff, limit concurrent reconciles per cluster, and surface misconfiguration via Status conditions so users can remediate.

**Threat:** Excessive backup frequency or large snapshot sizes exhaust object storage quotas or saturate network bandwidth.

**Mitigation:** Validate backup schedules and document recommended limits; make backup settings configurable and observable.

### F. Repudiation

**Threat:** Lack of audit trail for Operator-initiated actions (e.g., upgrades, step-downs, backups).

**Mitigation:**

- The Operator emits structured audit logs for critical operations (initialization, leader step-down) tagged with `audit=true` and `event_type` for easy filtering in log aggregation systems.
- Use structured, timestamped logs including `OpenBaoCluster` namespace/name and correlation IDs.
- Rely on Kubernetes/audit logs and OpenBao audit logs (configurable via `spec.audit`) where available.

## 3. Secrets Management Requirements

- **Requirement:** The CA Key generated by the Operator must never be logged to stdout/stderr.
- **Requirement:** The root token and initialization response (containing unseal keys) must NEVER be logged.
- **Requirement:** Credentials and tokens used for object storage access must be stored in Kubernetes Secrets or via workload identity, never embedded in ConfigMaps or images.
- **Requirement:** Secret names must be unique per `OpenBaoCluster` to prevent accidental cross-tenant sharing.
- **Requirement:** The root token Secret (`<cluster>-root-token`) must be protected with strict RBAC; access should be limited to cluster administrators and the Operator.
- **Recommendation:** Organizations should revoke or rotate the root token after initial cluster setup, storing it securely offline for emergency recovery only.
