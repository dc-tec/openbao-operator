---
description: Cross-cutting invariants preserved by OpenBao Operator across identity boundaries, production posture, configuration ownership, and lifecycle safety.
---

# Operator Invariants

This page defines the cross-cutting guarantees OpenBao Operator tries to preserve. Use it as the conceptual anchor for architecture, security, and lifecycle discussions.

!!! note "Stable Intent"
    These invariants describe what the operator is trying to preserve across releases. Implementation details may change, but weakening an invariant should be treated as a deliberate contract change.

## Identity and Access Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Provisioner and Controller identities remain separated** | Prevent one long-running identity from both minting and consuming tenant access. | Split ServiceAccounts, RBAC boundaries, admission policies. | [RBAC Architecture](../security/infrastructure/rbac.md) |
| **Tenant namespace access is introduced explicitly, not discovered** | Keep tenant onboarding intentional and prevent broad namespace discovery. | `OpenBaoTenant` onboarding flow, RoleBinding introduction, no namespace list for Controller. | [Tenant Isolation](../security/multi-tenancy/tenant-isolation.md) |
| **Secret access is name-scoped and non-enumerating** | Prevent broad tenant secret exposure and reduce blast radius. | Allowlisted Secret roles, no Secret list/watch in the normal model, admission policy restrictions. | [RBAC Architecture](../security/infrastructure/rbac.md) |
| **Admission enforcement is part of the normal safety model** | Keep API-level guardrails active before invalid or unsafe objects persist. | `ValidatingAdmissionPolicy` inventory, fail-closed startup, optional enforcement canary. | [Admission Policies](../security/infrastructure/admission-policies.md) |

## Production Posture Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Hardened production posture requires self-init, non-static unseal, and trusted TLS** | Prevent root token Secret persistence and weak bootstrap or transport defaults in production. | `openbao-validate-openbaocluster`, `ProductionReady` condition, Hardened profile validation. | [Security Profiles](../security/fundamentals/profiles.md) |
| **`OperatorManaged` TLS is not a Hardened production path** | Keep production posture aligned with external or OpenBao-native certificate trust models. | Admission validation for Hardened clusters, `ProductionReady=False` when `OperatorManaged` TLS is selected. | [TLS & Identity](../security/workload/tls.md) |
| **Self-init is the supported production bootstrap path** | Keep production bootstrap declarative and avoid storing the initial root token in a Secret. | Hardened profile requirements, validation policy, `ProductionReady` evaluation. | [Self-Initialization](../user-guide/openbaocluster/configuration/self-init.md) |
| **Operator-owned configuration stays operator-owned** | Preserve correctness for networking, storage, seal configuration, and listener identity. | Managed configuration rendering and admission ownership rules. | [Admission Policies](../security/infrastructure/admission-policies.md) |
| **`ProductionReady` means Hardened posture checks passed** | Keep status conditions narrowly scoped and avoid implying support or API stability guarantees. | Status condition evaluation and warning reasons. | [Status Conditions and Events](../reference/status-and-events.md) |

## Lifecycle Safety Invariants

| Invariant | Why it exists | Primary enforcement | Reference |
| :--- | :--- | :--- | :--- |
| **Only one disruptive operation owns the cluster operation lock at a time** | Prevent upgrades, backups, and restores from colliding on the same cluster. | `status.operationLock`, shared operation lifecycle coordination, manager-specific lock handling. | [Operation Lifecycle](operation-lifecycle.md) |
| **Restore remains destructive, explicit, and lock-aware** | Make disaster recovery visible and prevent restore from being treated as a routine reconcile side effect. | `OpenBaoRestore` CRD, restore validation, break-glass override, operation lock checks. | [Restore](../user-guide/openbaorestore/restore.md) |
| **OpenBao remains the source of truth for data consistency and snapshot semantics** | Keep the operator focused on orchestration and guardrails rather than reimplementing OpenBao data-plane behavior. | Supervisor Pattern, OpenBao API-driven backup and restore flows, OpenBao-led snapshot semantics. | [Architecture Overview](index.md) |

## Using This Page

!!! tip "When a change is high risk"
    If a change weakens one of these invariants, update the related architecture, security, and user-guide pages in the same change set. Treat it as a contract change, not only an implementation change.

## See Also

- [Architecture Overview](index.md)
- [Deployment Decision Guide](../user-guide/deployment-decision-guide.md)
- [Security Overview](../security/index.md)
