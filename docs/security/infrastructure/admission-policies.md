# Admission Policies

The Operator uses Kubernetes `ValidatingAdmissionPolicy` to enforce security invariants at admission time.

## Policies Shipped With The Operator

The operator ships several admission policies under `config/policy/` to enforce “GitOps-safe” and “least privilege” boundaries:

- **Managed Resource Lockdown (`lock-managed-resource-mutations`)**: Blocks direct CREATE/UPDATE/DELETE of operator-managed resources (labeled `app.kubernetes.io/managed-by=openbao-operator`). Users must change the parent `OpenBaoCluster` / `OpenBaoTenant` instead. A small set of exceptions exist for Kubernetes control-plane components, cert-manager, OpenBao service registration Pod label updates, and explicit break-glass maintenance.
- **Controller Self-Lockdown (`lock-controller-statefulset-mutations`)**: Defense-in-depth guard that prevents the OpenBaoCluster controller itself from mutating high-risk `StatefulSet` fields (volumes, commands/args, security context, token mounting, and volume mounts).
- **OpenBaoCluster Validation (`validate-openbaocluster`)**: Enforces baseline spec invariants (for example Hardened profile requirements, self-init request constraints, gateway requirements, backup configuration requirements, and backup endpoint SSRF protections).
- **Sentinel Hardening (`restrict-sentinel-mutations`)**: Restricts Sentinel to status-only trigger writes.
- **Provisioner Delegation Hardening (`restrict-provisioner-delegate`)**: Restricts what the impersonated delegate can create/bind when provisioning tenant RBAC.

## ValidatingAdmissionPolicy for Sentinel

A dedicated `ValidatingAdmissionPolicy` (`openbao-restrict-sentinel-mutations`) hardens the Sentinel drift detector:

- **Subject Scoping:** The policy only applies when the caller is the Sentinel ServiceAccount (`system:serviceaccount:<tenant-namespace>:openbao-sentinel`).
- **Status-Only Trigger:** The Sentinel is only allowed to update `status.sentinel` trigger fields (`triggerID`, `triggeredAt`, `triggerResource`).
- **Spec & Metadata Protection:** For Sentinel requests, `object.spec` and key metadata fields (labels/annotations/finalizers) must match `oldObject`.
- **All Other Status Blocked:** Any attempt by Sentinel to modify other status fields (conditions, phase, backup/upgrade state) is rejected.

**Result:** Even if the Sentinel binary is compromised, it can only signal drift via `status.sentinel` and cannot escalate privileges or mutate configuration.

!!! important "Required Dependency"
    These admission policies are not optional. If the Operator cannot verify the Sentinel mutation policy is installed and correctly bound, it will treat Sentinel as unsafe and will not deploy Sentinel. The supported Kubernetes baseline for these controls is v1.33+.

!!! note "Kustomize Name Prefixes"
    When deploying via `config/default`, `kustomize` applies a `namePrefix` (for example, `openbao-operator-`) to cluster-scoped admission resources. Ensure any referenced policy names/bindings match the rendered names.

## Configuration Ownership (Operator-Owned Stanzas)

The operator’s `config.hcl` is generated from typed CRD fields and operator-owned stanzas:

- **Operator-Owned By Construction:** The operator always renders core stanzas like `listener "tcp"`, `storage "raft"`, and `seal` based on `spec.*` (and renders fixed identity attributes like `api_addr`/`cluster_addr`).
- **User-Tunable Fields Are Typed:** User configuration under `spec.configuration` is a typed object (CRD schema). This is not a generic “accept arbitrary OpenBao HCL keys” escape hatch.

## Immutability

Certain security-critical fields are enforced at admission time via `validate-openbaocluster`, for example:

- Hardened profile requirements (TLS mode, external unseal, self-init enabled, disallowing `tlsSkipVerify=true` in supported seal configs)
- Init container required and enabled (and must specify an image)
- Self-init request constraints (non-empty, bounded list with unique names)
- Gateway and backup configuration invariants

## RBAC Delegation Hardening

The Provisioner uses a dedicated delegate ServiceAccount and impersonation to create tenant-scoped Roles and RoleBindings. This delegation is hardened at two layers:

- **Kubernetes RBAC Escalation Prevention:** The delegate ClusterRole (`openbao-operator-tenant-template`) defines the maximum permissions that can ever be granted to tenants. The API server enforces that the delegate cannot create Roles with permissions it does not already possess.
- **ValidatingAdmissionPolicy Guard:** A cluster-scoped `ValidatingAdmissionPolicy` (`openbao-restrict-provisioner-delegate`) restricts the Provisioner Delegate to:
  - Only creating specific Roles/RoleBindings by name.
  - Only binding those Roles to operator- and Sentinel-owned ServiceAccounts.
  - Rejecting any delegated Role whose `rules.verbs` include `impersonate`, `bind`, `escalate`, or `*`.

Together, these controls provide defense-in-depth: even if the delegate's ClusterRole is accidentally broadened, the policy still prevents impersonation or wildcard privilege grants through delegated Roles.

## Backup Endpoint SSRF Protection

To prevent Server-Side Request Forgery (SSRF) attacks, backup endpoint URLs are validated via `validate-openbaocluster`:

- **Blocked Endpoints:**
  - `localhost`, `127.0.0.1`, `::1` (loopback)
  - `169.254.x.x` (link-local addresses, including cloud metadata services like `169.254.169.254`)
- **Hardened Profile:** Requires HTTPS or S3 scheme for backup endpoints.

**Note:** Runtime checks still exist for other backup safety properties (for example cross-namespace SecretRef protections), but SSRF endpoint validation is enforced at admission.
