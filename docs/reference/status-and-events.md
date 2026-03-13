---
description: Reference for OpenBao Operator status conditions, Kubernetes Events, and audit events for OpenBaoCluster, OpenBaoRestore, and OpenBaoTenant resources.
---

# Status Conditions and Events

Use this reference to interpret status conditions and lifecycle events emitted by the OpenBao Operator.

## Inspect Resources

Run these commands to inspect CRD conditions:

```bash
kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaorestore <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaotenant <name> -o jsonpath='{.status.conditions}' | jq
```

!!! tip "Fastest Timeline View"
    Run `kubectl -n <ns> describe openbaocluster <name>`, `kubectl -n <ns> describe openbaorestore <name>`, or `kubectl -n <ns> describe openbaotenant <name>` to view status and recent events together.

Run this command to inspect recent namespace events:

```bash
kubectl -n <ns> get events --sort-by=.lastTimestamp
```

## OpenBaoCluster Conditions

Condition types defined in `api/v1alpha1`:

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `Available` | Workload availability from ready replicas | `AllReplicasReady`, `NoReplicasReady`, `NotReady`, `Paused` |
| `APIServerNetworkReady` | Operator-known Kubernetes API egress contract for operator-managed NetworkPolicies | `APIServerNetworkReady`, `APIServerEndpointIPsRecommended`, `APIServerNetworkConfigurationInvalid`, `Paused` |
| `TLSReady` | TLS asset readiness | `Ready`, `Disabled`, `TLSSecretMissing`, `TLSSecretInvalid`, `Unknown`, `Paused` |
| `ACMEIntegrationReady` | Operator-known ACME prerequisites such as Gateway passthrough, private ACME trust, and supported self-reachability checks | `ACMEIntegrationReady`, `GatewayAPIMissing`, `ACMEGatewayNotConfiguredForPassthrough`, `ACMEDomainNotResolvable`, `PrerequisitesMissing`, `Unknown`, `Paused` |
| `ACMECacheReady` | Shared ACME cache readiness for HA or blue/green ACME topologies | `ACMECacheReady`, `ACMECacheNotConfigured`, `ACMECacheMissing`, `ACMECachePending`, `ACMECacheInvalidAccessMode` |
| `GatewayIntegrationReady` | Operator-known Gateway API prerequisites and controller support for `spec.gateway` | `GatewayIntegrationReady`, `GatewayAPIMissing`, `GatewayReferenceMissing`, `GatewayClassMissing`, `GatewayClassPending`, `GatewayClassNotAccepted`, `GatewayVersionUnsupported`, `GatewayFeatureUnsupported`, `GatewayCapabilitiesUnknown`, `GatewayNotProgrammed`, `GatewayProgrammingPending`, `GatewayListenerIncompatible`, `Paused` |
| `BackupConfigurationReady` | Operator-known backup Job prerequisites such as auth references, storage credential references, hardened-profile egress rules, and job-specific identity assumptions | `Ready`, `AuthenticationRequired`, `TokenSecretMissing`, `CredentialsSecretMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `NetworkEgressRulesRequired`, `Unknown`, `Paused` |
| `CloudUnsealIdentityReady` | Operator-known authentication path for cloud KMS unseal on the main OpenBao Pods | `Ready`, `CredentialsSecretMissing`, `PrerequisitesMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `Unknown`, `Paused` |
| `ProductionReady` | Indicates whether the cluster currently meets the operator's Hardened production posture checks. This condition does not represent API stability or project support level. | `ProductionReady`, `ProfileNotSet`, `DevelopmentProfile`, `AdmissionPoliciesNotReady`, `OperatorManagedTLS`, `StaticUnsealInUse`, `RootTokenStored`, Gateway/ACME readiness reasons such as `GatewayFeatureUnsupported` or `ACMEGatewayNotConfiguredForPassthrough` |
| `Upgrading` | Upgrade state | `InProgress`, `Idle`, or upgrade failure reason |
| `BackingUp` | Backup job state | `InProgress`, `Idle` |
| `StorageConfigured` | Persistent storage class selection visibility | `StorageClassConfigured`, `StorageClassPending`, `StorageClassDefaulted`, `StorageClassUnset`, `StorageClassMismatch`, `StorageClassInconsistent` |
| `Degraded` | Problem requiring attention | `BreakGlassRequired`, upgrade failure reason, workload/adminops error reason, `OIDCBootstrapConfigurationInvalid`, `APIServerNetworkConfigurationInvalid`, `RootTokenStored`, `Reconciling`, `Paused` |
| `EtcdEncryptionWarning` | etcd encryption verification warning | `EtcdEncryptionUnknown` |
| `SecurityRisk` | Relaxed security mode indicator | `DevelopmentProfile` |
| `OpenBaoInitialized` | OpenBao initialization observed from registration labels | `Initialized`, `NotInitialized`, `Unknown` |
| `OpenBaoSealed` | OpenBao seal state observed from registration labels | `Sealed`, `Unsealed`, `Unknown` |
| `OpenBaoLeader` | Leader discovery from registration labels | `LeaderFound`, `LeaderUnknown`, `MultipleLeaders` |
| `NodeSecurityCapabilityMismatch` | Node capability mismatch for enabled hardening | `Ready`, `AppArmorUnsupported` |

## OpenBaoRestore Conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `RestoreComplete` | Restore terminal state | `RestoreSucceeded`, `RestoreFailed`, `AuthenticationRequired` |
| `RestoreConfigurationReady` | Operator-known restore prerequisites such as auth references, storage credential references, hardened-profile egress rules, and job-specific identity assumptions | `Ready`, `AuthenticationRequired`, `TokenSecretMissing`, `CredentialsSecretMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `NetworkEgressRulesRequired` |
| `OperationLockOverride` | Break-glass lock override occurred | `OperationLockOverridden` |

## OpenBaoTenant Conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `Provisioned` | Tenant RBAC provisioning state | `SecurityViolation` (guardrail block) and provisioning outcomes |

## Kubernetes Events

!!! note "Event Scope"
    The operator emits lifecycle events on parent custom resources only. `OpenBaoCluster` receives cluster lifecycle, init/bootstrap, upgrade, backup, and tenant Secret RBAC sync events. `OpenBaoRestore` receives restore lifecycle events. `OpenBaoTenant` receives tenant provisioning lifecycle events. Jobs do not receive the lifecycle events listed here.

!!! tip "Event Types"
    Expect `Normal` events for routine progression and accepted operator input. Expect `Warning` events for failures, contention, overrides, and other states that need attention.

### OpenBaoCluster Safety and Maintenance Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Warning` | `ProfileNotSet` | `spec.profile` missing; reconciliation blocked. |
| `Warning` | `DevelopmentProfile` | Development profile warning for production. |
| `Normal` | `AmbientUnsealIdentity` | Cloud KMS unseal is relying on ambient identity or the provider default chain for the main OpenBao Pods. This note is emitted only when the operator is not using a credentials Secret or explicit inline cloud credentials. |
| `Warning` | `StaticUnsealInUse` | Static unseal warning. |
| `Warning` | `RootTokenStored` | Self-init is disabled and the operator stored the root token Secret. |
| `Warning` | `ImageVerificationFailed` and related reasons | Warn-policy image verification failures. |
| `Normal` | `PVCResize` | PVC expansion started. |
| `Normal` | `PVCResizeLeaderStepDown` | Leader step-down for resize restart path. |
| `Normal` | `PVCResizePodRestart` | Pod restart to complete filesystem resize. |

### OpenBaoCluster Init and Bootstrap Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `InitStarted` | Self-init or operator-driven initialization started or is still in progress. |
| `Normal` | `InitCompleted` | Cluster initialization completed successfully. |
| `Warning` | `InitFailed` | Operator-driven initialization failed. |

### OpenBaoCluster Tenant Secret RBAC Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `TenantSecretRBACSynchronized` | Tenant Secret RBAC allowlists were synchronized for the namespace. |

### OpenBaoCluster Upgrade Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `UpgradeStarted` | Upgrade orchestration started. |
| `Normal` | `PreUpgradeSnapshotJobCreated` | Pre-upgrade snapshot Job created. |
| `Normal` | `PreUpgradeSnapshotCompleted` | Pre-upgrade snapshot completed successfully. |
| `Warning` | `PreUpgradeSnapshotFailed` | Pre-upgrade snapshot failed and upgrade is blocked. |
| `Normal` | `RollingRetryRequested` | Manual retry requested for a failed rolling upgrade. |
| `Normal` | `RollingRetryAccepted` | Failed rolling upgrade state cleared and retry resumed. |
| `Normal` | `BlueGreenHoldEntered` | Blue/green upgrade is waiting for manual promotion approval. |
| `Normal` | `BlueGreenPromotionApproved` | Promotion approval observed and promotion started. |
| `Normal` | `UpgradeComplete` | Upgrade finished successfully. |
| `Warning` | `UpgradeFailed` | Upgrade failed and operator marked the upgrade as failed. |
| `Warning` | `RollbackStarted` | Blue/green rollback started. |
| `Warning` | `BreakGlassEntered` | Blue/green rollback entered break-glass mode. |
| `Normal` | `BreakGlassAcknowledged` | Break-glass mode was acknowledged and automation may resume. |
| `Warning` | `OperationLockBlocked` | Upgrade is waiting for another cluster operation to release the lock. |

### OpenBaoCluster Backup Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `BackupManualTriggerAccepted` | Manual backup trigger accepted. |
| `Normal` | `BackupSkipped` | Due or manually requested backup intentionally skipped. |
| `Normal` | `BackupStarted` | Backup attempt started after lock acquisition. |
| `Normal` | `BackupIdentityConfiguration` | Backup identity mode and generated ServiceAccount attachment point. |
| `Normal` | `BackupJobCreated` | Backup Job created. |
| `Normal` | `BackupCompleted` | Backup completed successfully. |
| `Warning` | `BackupFailed` | Backup Job failed. |
| `Warning` | `OperationLockBlocked` | Backup is waiting for another cluster operation to release the lock. |

### OpenBaoRestore Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `RestoreValidationStarted` | Restore validation started. |
| `Normal` | `RestoreStarted` | Restore execution started after validation. |
| `Normal` | `RestoreIdentityConfiguration` | Restore identity mode and generated ServiceAccount attachment point. |
| `Normal` | `RestoreJobCreated` | Restore Job created. |
| `Normal` | `RestoreCompleted` | Restore completed successfully. |
| `Warning` | `RestoreFailed` | Restore failed. |
| `Warning` | `OperationLockBlocked` | Restore is waiting for another cluster operation to release the lock. |
| `Warning` | `OperationLockLost` | Restore lost the cluster operation lock while running. |
| `Warning` | `OperationLockOverride` | Lock override requested with break-glass restore. |

### OpenBaoTenant Provisioning Events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `TenantProvisioned` | Tenant namespace RBAC was provisioned successfully. |
| `Normal` | `TenantRBACCleaned` | Tenant namespace RBAC was cleaned up during deletion. |
| `Warning` | `TenantProvisioningBlocked` | Provisioning is blocked by guardrails, missing prerequisites, or dependency readiness checks. |
| `Warning` | `TenantProvisioningFailed` | Provisioning failed while applying tenant RBAC. |

## Structured Audit Events

In addition to Kubernetes Events, controllers emit structured audit events to logs, for example `UpgradeStarted`, `UpgradeFailed`, `BackupJobCreated`, `RestoreCompleted`, and `TenantRBACProvisioned`.

Query centralized logs for these high-signal lifecycle events.

!!! note "Stability"
    Condition **types** are part of the API surface. Reason and event values may expand over time as new scenarios are added.
