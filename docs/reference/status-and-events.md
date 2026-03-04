---
description: Reference for OpenBao Operator status conditions and common Kubernetes/audit events used for troubleshooting and automation.
---

# Status Conditions & Events Reference

Use this document to interpret CRD status and controller-emitted events.

## 1. Inspect Status and Events

Check CRD conditions:

```bash
kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaorestore <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaotenant <name> -o jsonpath='{.status.conditions}' | jq
```

Check recent events:

```bash
kubectl -n <ns> get events --sort-by=.lastTimestamp
```

## 2. OpenBaoCluster Conditions

Condition types defined in `api/v1alpha1`:

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `Available` | Workload availability from ready replicas | `AllReplicasReady`, `NoReplicasReady`, `NotReady`, `Paused` |
| `TLSReady` | TLS asset readiness | `Ready`, `Disabled`, `TLSSecretMissing`, `TLSSecretInvalid`, `Unknown`, `Paused` |
| `ProductionReady` | Hardened production posture validation | `ProductionReady`, `ProfileNotSet`, `DevelopmentProfile`, `AdmissionPoliciesNotReady`, `OperatorManagedTLS`, `StaticUnsealInUse`, `RootTokenStored` |
| `Upgrading` | Upgrade state | `InProgress`, `Idle`, or upgrade failure reason |
| `BackingUp` | Backup job state | `InProgress`, `Idle` |
| `Degraded` | Problem requiring attention | `BreakGlassRequired`, upgrade failure reason, workload/adminops error reason, `RootTokenStored`, `Reconciling`, `Paused` |
| `EtcdEncryptionWarning` | etcd encryption verification warning | `EtcdEncryptionUnknown` |
| `SecurityRisk` | Relaxed security mode indicator | `DevelopmentProfile` |
| `OpenBaoInitialized` | OpenBao initialization observed from registration labels | `Initialized`, `NotInitialized`, `Unknown` |
| `OpenBaoSealed` | OpenBao seal state observed from registration labels | `Sealed`, `Unsealed`, `Unknown` |
| `OpenBaoLeader` | Leader discovery from registration labels | `LeaderFound`, `LeaderUnknown`, `MultipleLeaders` |
| `NodeSecurityCapabilityMismatch` | Node capability mismatch for enabled hardening | `Ready`, `AppArmorUnsupported` |

## 3. OpenBaoRestore Conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `RestoreComplete` | Restore terminal state | `RestoreSucceeded`, `RestoreFailed`, `AuthenticationRequired` |
| `OperationLockOverride` | Break-glass lock override occurred | `OperationLockOverridden` |

## 4. OpenBaoTenant Conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `Provisioned` | Tenant RBAC provisioning state | `SecurityViolation` (guardrail block) and provisioning outcomes |

## 5. Common Kubernetes Events

The operator emits Kubernetes Events for selected actions:

| Resource | Type | Reason | Notes |
| :--- | :--- | :--- | :--- |
| `OpenBaoCluster` | `Warning` | `ProfileNotSet` | `spec.profile` missing; reconciliation blocked. |
| `OpenBaoCluster` | `Warning` | `DevelopmentProfile` | Development profile warning for production. |
| `OpenBaoCluster` | `Warning` | `StaticUnsealInUse` | Static unseal warning. |
| `OpenBaoCluster` | `Warning` | `RootTokenStored` | SelfInit disabled; root token secret warning. |
| `OpenBaoCluster` | `Warning` | Image verification reasons | For warn-policy image verification failures (for example `ImageVerificationFailed`). |
| `OpenBaoCluster` | `Normal` | `PVCResize` | PVC expansion started. |
| `OpenBaoCluster` | `Normal` | `PVCResizeLeaderStepDown` | Leader step-down for resize restart path. |
| `OpenBaoCluster` | `Normal` | `PVCResizePodRestart` | Pod restart to complete filesystem resize. |
| `OpenBaoRestore` | `Warning` | `OperationLockOverride` | Lock override requested with break-glass restore. |

## 6. Structured Audit Events (Controller Logs)

In addition to Kubernetes Events, controllers emit structured audit events to logs (for example `UpgradeStarted`, `UpgradeFailed`, `BackupJobCreated`, `RestoreCompleted`, `TenantRBACProvisioned`).

Use centralized logs to query these high-signal lifecycle events.

!!! note "Stability"
    Condition **types** are part of the API surface. Reason and event values may expand over time as new scenarios are added.
