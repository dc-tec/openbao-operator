# Tenant Isolation (Namespace Provisioner)

The Operator includes a **Namespace Provisioner** controller that manages RBAC for tenant namespaces using a governance model based on the `OpenBaoTenant` Custom Resource Definition (CRD).

## Governance Model (OpenBaoTenant CRD)

Instead of watching all namespaces for labels, the Provisioner watches `OpenBaoTenant` CRDs that explicitly declare which namespaces should be provisioned. This eliminates the need for `list` and `watch` permissions on namespaces, significantly improving the security posture.

- **OpenBaoTenant CRD:** A namespaced resource that declares a target namespace for provisioning. Users create an `OpenBaoTenant` resource in the operator's namespace (typically `openbao-operator-system`) with `spec.targetNamespace` set to the namespace that should receive tenant RBAC.
- **Tenant Role (`openbao-operator-tenant-role`):** A namespace-scoped Role is created in each target namespace granting the OpenBaoCluster Controller ServiceAccount permission to manage OpenBao-related resources (StatefulSets, Secrets, Services, ConfigMaps, etc.) in that specific namespace only.
- **Tenant RoleBinding (`openbao-operator-tenant-rolebinding`):** A RoleBinding ties the OpenBaoCluster Controller ServiceAccount to the tenant Role, ensuring it only operates on tenant resources within their designated boundary.

## Security Benefits

- The OpenBaoCluster controller cannot access resources in non-tenant namespaces.
- The OpenBaoCluster controller cannot access resources unrelated to OpenBao in tenant namespaces (e.g., other applications' Secrets, Deployments, etc.).
- The Provisioner cannot access workload resources directly, only create Roles/RoleBindings.
- **Information Disclosure Mitigation:** The Provisioner cannot enumerate namespaces to discover cluster topology. It can only access namespaces explicitly declared in `OpenBaoTenant` CRDs.
- **Elevation of Privilege Mitigation:** Creating an `OpenBaoTenant` CRD requires write access to the operator's namespace, which should be highly restricted (Cluster Admin only).

**Recommendation:** Tenants should typically be granted `edit` or `view` roles for `OpenBaoCluster` resources but should **not** have `get` access to Secrets matching `*-root-token` or `*-unseal-key`. Additionally, only cluster administrators should have permission to create `OpenBaoTenant` resources.

## Tenant Namespace Provisioning Flow

1. User creates a namespace (target namespace for OpenBao clusters)
2. User creates an `OpenBaoTenant` CRD in the operator's namespace (typically `openbao-operator-system`) with `spec.targetNamespace` set to the target namespace
3. Provisioner controller detects the `OpenBaoTenant` CRD via watch
4. Provisioner verifies the target namespace exists (using `get` permission)
5. Provisioner creates the tenant Role (`openbao-operator-tenant-role`) in the target namespace
6. Provisioner creates the tenant RoleBinding (`openbao-operator-tenant-rolebinding`) binding the `openbao-operator-controller` ServiceAccount to the tenant Role
7. Provisioner applies Pod Security Standards labels to the namespace:
   - `pod-security.kubernetes.io/enforce: restricted`
   - `pod-security.kubernetes.io/audit: restricted`
   - `pod-security.kubernetes.io/warn: restricted`
8. Provisioner updates `OpenBaoTenant.Status.Provisioned = true`
9. OpenBaoCluster controller can now manage OpenBaoCluster resources in that namespace

**Note:** If the target namespace does not exist, the Provisioner updates `OpenBaoTenant.Status.LastError` and requeues the reconciliation. The Provisioner will retry once the namespace is created.

## Security Benefits of Split-Controller Design

**Provisioner Compromise:**

- Cannot read or modify workload resources (StatefulSets, Secrets, Services, etc.) in tenant namespaces
- Can only create Roles/RoleBindings, which is a limited attack surface
- Cannot access OpenBao data or configuration

**OpenBaoCluster Controller Compromise:**

- Cannot access resources outside of tenant namespaces
- Cannot access non-OpenBao resources within tenant namespaces (unless explicitly granted)
- Cannot create or modify cluster-wide resources
- Cannot access other tenants' namespaces

## Tenant Isolation

- Each tenant namespace has its own Role and RoleBinding
- The OpenBaoCluster controller can only access resources in namespaces where it has been granted permissions
- Cross-tenant access is impossible without explicit RoleBindings

## Multi-Tenancy Security Considerations

In multi-tenant deployments, additional threats and mitigations apply:

### Tenant Isolation Threats

- **Threat:** Tenant A reads Tenant B's root token or unseal key Secret.
- **Mitigation:**
  - **Namespace-Scoped Permissions:** The OpenBaoCluster controller uses namespace-scoped Roles that only grant permissions within the tenant namespace, preventing cross-namespace access.
  - Use namespace-scoped RoleBindings that do NOT grant Secret read access to tenants.
  - Deploy policy engines (OPA Gatekeeper, Kyverno) to block access to `*-root-token` and `*-unseal-key` Secrets.
  - **External KMS:** When using external KMS auto-unseal (`spec.unseal.type` set to `awskms`, `gcpckms`, `azurekeyvault`, or `transit`), the unseal key is not stored in Kubernetes Secrets, eliminating this threat vector for the unseal key.
  - Consider using self-initialization to avoid root token Secrets entirely.

- **Threat:** Tenant A's OpenBao pods communicate with Tenant B's pods.
- **Mitigation:** The Operator automatically creates NetworkPolicies for each cluster that restrict ingress/egress to pods with matching `openbao.org/cluster` labels, enforcing default-deny-all-ingress with exceptions for same-cluster pods, kube-system, and operator pods.

- **Threat:** Tenant A accesses Tenant B's backup data in object storage.
- **Mitigation:**
  - Each tenant MUST have separate backup credentials.
  - IAM policies MUST restrict access to tenant-specific bucket prefixes.
  - Backup credentials should be write-only (read access granted separately for restore operations).

### Resource Exhaustion Threats

- **Threat:** One tenant creates excessive OpenBaoCluster resources, exhausting cluster resources.
- **Mitigation:** Configure ResourceQuotas per namespace to limit pods, PVCs, CPU, and memory. Controller rate limiting (MaxConcurrentReconciles: 3) prevents one cluster from starving the reconciler.

### Recommended Multi-Tenancy Controls

For secure multi-tenant deployments, platform teams SHOULD implement:

| Control | Purpose | Implementation |
| :--- | :--- | :--- |
| Separate ServiceAccounts | Least-privilege operator permissions | Provisioner ServiceAccount (RBAC management only) and OpenBaoCluster Controller ServiceAccount (tenant-scoped only) |
| Namespace-scoped RBAC | Isolate tenant access to CRs | RoleBindings with `openbaocluster-editor-role`; tenant Roles created by Provisioner |
| Secret access restrictions | Protect root tokens and unseal keys | Policy engine rules or explicit RBAC denies |
| NetworkPolicies | Isolate cluster network traffic | Automatically created by Operator (default deny all ingress, allow same-cluster and kube-system) |
| ResourceQuotas | Prevent resource exhaustion | Namespace-level quotas |
| Backup credential isolation | Protect backup data | Per-tenant IAM credentials with prefix restrictions |
| Pod Security Standards | Enforce secure pod configuration | PSA labels on namespaces |
| Audit logging | Monitor sensitive access | Kubernetes audit policy for Secrets |

**Note:** The Operator implements the separate ServiceAccounts model by default. The Provisioner ServiceAccount has minimal cluster-wide permissions (namespace `get`/`update`/`patch` + OpenBaoTenant CRD management + RBAC management), while the OpenBaoCluster Controller ServiceAccount has cluster-wide read access to `openbaoclusters` (`get;list;watch`) and cluster-wide `create` on `tokenreviews`/`subjectaccessreviews` (for the protected metrics endpoint). All OpenBaoCluster writes and all child resources are tenant-scoped via namespace Roles created by the Provisioner. The Provisioner uses a governance model based on `OpenBaoTenant` CRDs, eliminating the need for `list`/`watch` permissions on namespaces and preventing cluster topology enumeration.
