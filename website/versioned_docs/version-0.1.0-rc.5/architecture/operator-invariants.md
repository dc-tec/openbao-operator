---
description: Cross-cutting invariants preserved by OpenBao Operator across identity boundaries, production posture, configuration ownership, and lifecycle safety.
---

# Operator Invariants

This page defines the cross-cutting guarantees OpenBao Operator tries to preserve. Use it as the conceptual anchor for architecture, security, and lifecycle discussions.

<Callout type="note" title="Lifecycle Contract">

`OpenBaoCluster` is an operator-owned lifecycle contract. It is not a generic import API for arbitrary unmanaged OpenBao clusters.

</Callout>

<Callout type="note" title="Stable Intent">

These invariants describe what the operator is trying to preserve across releases. Implementation details may change, but weakening an invariant should be treated as a deliberate contract change.

</Callout>

## Identity and Access Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Provisioner and Controller identities remain separated** | Prevent one long-running identity from both minting and consuming tenant access. | Split ServiceAccounts, RBAC boundaries, admission policies. | [RBAC Architecture](../security/infrastructure/rbac.md) |
| **Rendered operator identities stay internally consistent** | Prevent raw-manifest installs from drifting between ServiceAccounts, RoleBindings, and admission policy subjects. | Helm values, raw-manifest overlays, admission-policy variables, install-time render tests. | [Operator Installation](../user-guide/operator/installation.md) |
| **Operator-managed identities and RBAC stay mutation-locked** | Prevent users, GitOps, or tenant workloads from drifting the ServiceAccounts, Roles, and RoleBindings that define operator access boundaries. | Managed-resource admission locks, RBAC restrictor policies, break-glass allowlist. | [Admission Policies](../security/infrastructure/admission-policies.md) |
| **Tenant namespace access is introduced explicitly, not discovered** | Keep tenant onboarding intentional and prevent broad namespace discovery. | `OpenBaoTenant` onboarding flow, RoleBinding introduction, no namespace list for Controller. | [Tenant Isolation](../security/multi-tenancy/tenant-isolation.md) |
| **Tenant guardrails remain provisioner-owned** | Preserve the Day 0 / Day 1 boundary so tenant quotas, limit ranges, and namespace labels do not drift into normal workload reconciliation. | Provisioner-owned onboarding flow, tenant governance admission policy, controller RBAC exclusions. | [RBAC Architecture](../security/infrastructure/rbac.md) |
| **Secret access is name-scoped and non-enumerating** | Prevent broad tenant secret exposure and reduce blast radius. | Allowlisted Secret roles, no Secret list/watch in the normal model, admission policy restrictions. | [RBAC Architecture](../security/infrastructure/rbac.md) |
| **Tenant Secret mutation remains blind-create and name-scoped** | Allow operator-managed Secret creation without enabling arbitrary tenant Secret discovery or mutation. | Dedicated Secret reader/writer Roles, name-scoped mutate verbs, admission policy restrictions on managed Secret writes. | [RBAC Architecture](../security/infrastructure/rbac.md) |
| **Admission enforcement is part of the normal safety model** | Keep API-level guardrails active before invalid or unsafe objects persist. | `ValidatingAdmissionPolicy` inventory, fail-closed startup, optional enforcement canary. | [Admission Policies](../security/infrastructure/admission-policies.md) |
| **Admission dependency loss pauses sensitive reconciliation** | Prevent the operator from continuing privileged writes after required guardrails disappear or stop applying. | Runtime admission tracker, fail-closed reconciliation gates, status conditions and degraded reasons. | [Status Conditions and Events](../reference/status-and-events.md) |

<Callout type="note" title="Identity Map">

For the compact bridge between Kubernetes identities, OpenBao authentication, and authorization surfaces, see [Operator Identity And Access](../user-guide/operator/identity-and-access.md).

</Callout>

## Production Posture Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Hardened production posture requires self-init, non-static unseal, and trusted TLS** | Prevent root token Secret persistence and weak bootstrap or transport defaults in production. | `openbao-validate-openbaocluster`, `ProductionReady` condition, Hardened profile validation. | [Security Profiles](../security/fundamentals/profiles.md) |
| **`OperatorManaged` TLS is not a Hardened production path** | Keep production posture aligned with external or OpenBao-native certificate trust models. | Admission validation for Hardened clusters, `ProductionReady=False` when `OperatorManaged` TLS is selected. | [TLS & Identity](../security/workload/tls.md) |
| **Self-init is the supported production bootstrap path** | Keep production bootstrap declarative and avoid storing the initial root token in a Secret. | Hardened profile requirements, validation policy, `ProductionReady` evaluation. | [Self-Initialization](../user-guide/openbaocluster/configuration/self-init.md) |
| **Operator-owned configuration stays operator-owned** | Preserve correctness for networking, storage, seal configuration, and listener identity. | Managed configuration rendering and admission ownership rules. | [Admission Policies](../security/infrastructure/admission-policies.md) |
| **`ProductionReady` means Hardened posture checks passed** | Keep status conditions narrowly scoped and avoid implying support or API stability guarantees. | Status condition evaluation and warning reasons. | [Status Conditions and Events](../reference/status-and-events.md) |

## Integration Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Gateway, ACME, and API-server assumptions surface as explicit conditions** | Turn environment and controller assumptions into operator-visible contracts before they become late runtime failures. | `GatewayIntegrationReady`, `ACMEIntegrationReady`, `ACMECacheReady`, and `APIServerNetworkReady`. | [Status Conditions and Events](../reference/status-and-events.md) |
| **Backup and restore identity remains separate from main workload identity** | Prevent day-2 Jobs from silently inheriting the wrong auth path or egress assumptions from the main StatefulSet. | Generated Job ServiceAccounts, backup and restore readiness evaluation, explicit status reasons. | [Backup Operations](../user-guide/openbaocluster/operations/backups.md) |

## Lifecycle Safety Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Only one disruptive operation owns the cluster operation lock at a time** | Prevent upgrades, backups, and restores from colliding on the same cluster. | `status.operationLock`, shared operation lifecycle coordination, manager-specific lock handling. | [Operation Lifecycle](operation-lifecycle.md) |
| **Restore remains destructive, explicit, and lock-aware** | Make disaster recovery visible and prevent restore from being treated as a routine reconcile side effect. | `OpenBaoRestore` CRD, restore validation, break-glass override, operation lock checks. | [Restore](../user-guide/openbaorestore/restore.md) |
| **Break-glass access remains explicit and narrow** | Preserve a deliberate maintenance escape hatch without turning administrative mutation into the normal operating model. | Configured maintenance break-glass groups, managed-resource admission exceptions, recovery and maintenance runbooks. | [Admission Policies](../security/infrastructure/admission-policies.md) |
| **OpenBao remains the source of truth for data consistency and snapshot semantics** | Keep the operator focused on orchestration and guardrails rather than reimplementing OpenBao data-plane behavior. | Supervisor Pattern, OpenBao API-driven backup and restore flows, OpenBao-led snapshot semantics. | [Architecture Overview](/docs/architecture) |

## Using This Page

<Callout type="tip" title="When a change is high risk">

If a change weakens one of these invariants, update the related architecture, security, and user-guide pages in the same change set. Treat it as a contract change, not only an implementation change.

</Callout>

## See Also

- [Architecture Overview](/docs/architecture)
- [Deployment Decision Guide](../user-guide/operator/deployment-decision-guide.md)
- [Security Overview](/docs/security)
