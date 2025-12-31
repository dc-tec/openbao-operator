# Admission Policies

The Operator uses Kubernetes `ValidatingAdmissionPolicy` to enforce security invariants at admission time.

## ValidatingAdmissionPolicy for Sentinel

A dedicated `ValidatingAdmissionPolicy` (`openbao-restrict-sentinel-mutations`) hardens the Sentinel drift detector:

- **Subject Scoping:** The policy only applies when the caller is the Sentinel ServiceAccount (`system:serviceaccount:<tenant-namespace>:openbao-sentinel`).
- **Spec & Status Protection:** For Sentinel requests, `object.spec` and `object.status` must match `oldObject.spec` and `oldObject.status`. The Sentinel cannot change desired state or status.
- **Metadata Restrictions:** The only metadata changes allowed are:
  - Adding or updating `openbao.org/sentinel-trigger`
  - Adding or updating `openbao.org/sentinel-trigger-resource`
- **All Other Changes Blocked:** Any attempt by Sentinel to modify labels, finalizers, or other annotations is rejected.

**Result:** Even if the Sentinel binary is compromised, it can only signal drift via the trigger annotations and cannot escalate privileges or mutate configuration.

!!! important "Required Dependency"
    These admission policies are not optional. If the Operator cannot verify the Sentinel mutation policy is installed and correctly bound, it will treat Sentinel as unsafe and will not deploy Sentinel. The supported Kubernetes baseline for these controls is v1.33+.

!!! note "Kustomize Name Prefixes"
    When deploying via `config/default`, `kustomize` applies a `namePrefix` (for example, `openbao-operator-`) to cluster-scoped admission resources. Ensure any referenced policy names/bindings match the rendered names.

## Configuration Allowlist

The operator rejects `OpenBaoCluster` resources that attempt to override protected configuration stanzas. This prevents tenants from weakening security controls managed by the operator.

- **Protected Stanzas:** Users cannot override `listener`, `storage`, `seal`, `api_addr`, or `cluster_addr` via `spec.configuration` or legacy `spec.config`. These are strictly owned by the operator to ensure mTLS and Raft integrity.
- **Parameter Allowlist:** Only known-safe OpenBao configuration parameters are accepted. Unknown or arbitrary keys are rejected by admission.

## Immutability

Certain security-critical fields, such as the enabling/disabling of the Init Container, are validated to ensure the cluster cannot be put into an unsupported or insecure state.

## RBAC Delegation Hardening

The Provisioner uses a dedicated delegate ServiceAccount and impersonation to create tenant-scoped Roles and RoleBindings. This delegation is hardened at two layers:

- **Kubernetes RBAC Escalation Prevention:** The delegate ClusterRole (`openbao-operator-tenant-template`) defines the maximum permissions that can ever be granted to tenants. The API server enforces that the delegate cannot create Roles with permissions it does not already possess.
- **ValidatingAdmissionPolicy Guard:** A cluster-scoped `ValidatingAdmissionPolicy` (`openbao-restrict-provisioner-delegate`) restricts the Provisioner Delegate to:
  - Only creating specific Roles/RoleBindings by name.
  - Only binding those Roles to operator- and Sentinel-owned ServiceAccounts.
  - Rejecting any delegated Role whose `rules.verbs` include `impersonate`, `bind`, `escalate`, or `*`.

Together, these controls provide defense-in-depth: even if the delegate's ClusterRole is accidentally broadened, the policy still prevents impersonation or wildcard privilege grants through delegated Roles.

## Backup Endpoint SSRF Protection

To prevent Server-Side Request Forgery (SSRF) attacks, backup endpoint URLs are validated via `ValidatingAdmissionPolicy`:

- **Blocked Endpoints:**
  - `localhost`, `127.0.0.1`, `::1` (loopback)
  - `169.254.x.x` (link-local addresses, including cloud metadata services like `169.254.169.254`)
- **Hardened Profile:** Requires HTTPS or S3 scheme for backup endpoints.
- **Defense-in-Depth:** Go-side validation in `BackupManager.checkPreconditions` provides runtime verification.
