# Operator Identity And Access

This page bridges the operator's Kubernetes identities, OpenBao authentication, and authorization boundaries.

Use it together with:

- [Installation](installation.md) for render-time identity checks
- [Authentication](authn.md) for the JWT audience and binding contract
- [Authorization](authz.md) for the OpenBao-side policies and roles
- [RBAC Architecture](../../security/infrastructure/rbac.md) for Kubernetes permission boundaries

## Identity Map

| Actor | Kubernetes identity | OpenBao auth | Primary authorization surface | Notes |
| :--- | :--- | :--- | :--- | :--- |
| **Provisioner** | Provisioner `ServiceAccount` in the operator namespace | None | Kubernetes RBAC only | Day 0 onboarding only. Writes tenant guardrails and tenant-scoped RBAC, but does not authenticate to OpenBao. |
| **Controller** | Controller `ServiceAccount` in the operator namespace | Projected JWT token -> `openbao-operator` role/policy | Kubernetes RBAC plus OpenBao maintenance policy | Long-running control-plane identity. Its rendered name, namespace, and JWT audience must stay internally consistent across manifests, RBAC, and OpenBao role binding. |
| **Main OpenBao Pods** | Per-cluster `ServiceAccount` in the tenant namespace | OpenBao server process auth and configured seal/unseal integration | Kubernetes workload identity plus OpenBao runtime configuration | Distinct from operator identities. Used for pod lifecycle, service registration, and OpenBao runtime concerns such as cloud or transit unseal. |
| **Backup Job** | Generated backup `ServiceAccount` in the tenant namespace | Projected JWT token or explicit backup token Secret | OpenBao snapshot policy plus backup-target credentials | Day-2 executor identity. Separate from the main StatefulSet identity and evaluated through `BackupConfigurationReady`. |
| **Restore Job** | Generated restore `ServiceAccount` in the tenant namespace | Projected JWT token or explicit restore token Secret | OpenBao restore policy plus restore-source credentials | Destructive executor identity. Separate from the main StatefulSet identity and evaluated through `RestoreConfigurationReady`. |
| **Upgrade Job** | Generated upgrade `ServiceAccount` in the tenant namespace | Projected JWT token | OpenBao upgrade policy | Used for rolling and blue/green upgrade orchestration. Does not inherit the controller identity. |

## Install-Sensitive Checks

Use these checks when you install the operator with raw manifests, a custom namespace, or a `namePrefix`.

1. Confirm the rendered controller `ServiceAccount` name and namespace.
2. Confirm the controller `Deployment` still mounts the projected `openbao-token`.
3. Confirm RoleBinding and admission-policy subjects point at the same rendered controller identity.
4. Confirm the operator installation audience `OPENBAO_JWT_AUDIENCE` matches the projected `openbao-token` audience.
5. Confirm the OpenBao JWT role binds to the same rendered controller identity.

For the render steps themselves, use [Installation](installation.md#render-verification).

## Common Failure Modes

| Symptom | Most likely boundary | What to verify |
| :--- | :--- | :--- |
| `permission denied` when the controller talks to OpenBao | Controller JWT auth or OpenBao role binding | [Authentication](authn.md#troubleshooting) |
| Custom raw-manifest install fails after namespace or prefix changes | Rendered identity drift | [Installation](installation.md#render-verification) |
| Backup or restore auth fails while the main cluster is healthy | Executor Job identity drift | [Authorization](authz.md), [Backups](../openbaocluster/operations/backups.md), [Restore](../openbaorestore/restore.md) |
| Tenant onboarding works, but controller access does not | Kubernetes RBAC / RoleBinding introduction | [RBAC Architecture](../../security/infrastructure/rbac.md) |

## See Also

- [Installation](installation.md)
- [Authentication](authn.md)
- [Authorization](authz.md)
- [RBAC Architecture](../../security/infrastructure/rbac.md)
