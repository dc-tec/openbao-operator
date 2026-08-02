---
title: Status Conditions and Events
description: Reference for OpenBao Operator status conditions, Kubernetes Events, and audit events for OpenBaoCluster, OpenBaoRestore, and OpenBaoTenant resources.
pageType: reference
journey: reference
---

<PageHeader
  title="Status conditions and event reference"
  lede="Status conditions and emitted events across `OpenBaoCluster`, `OpenBaoRestore`, and `OpenBaoTenant`."
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect status conditions and namespace events"
  code={`kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaorestore <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaotenant <name> -o jsonpath='{.status.conditions}' | jq

kubectl -n <ns> get events --sort-by=.lastTimestamp`}
>
  For the fastest timeline view, run `kubectl describe` on the parent custom resource to see status and recent events together.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Workflow checkpoints"
  caption="Condition sets for quick checks of common workflows."
  columns={['Workflow', 'Conditions to watch']}
  rows={[
    {
      cells: ['Hardened with external TLS', '`Available`, `TLSReady`, `UserAccessBootstrap`, `ProductionReady`'],
      emphasis: 'recommended',
    },
    {
      cells: ['Hardened with ACME', '`Available`, `ACMEIntegrationReady`, `ACMECacheReady`, `UserAccessBootstrap`, `ProductionReady`'],
    },
    {
      cells: ['Gateway exposure', '`GatewayIntegrationReady`'],
    },
    {
      cells: ['Strict NetworkPolicy environments', '`APIServerNetworkReady`'],
    },
    {
      cells: ['Scheduled backups', '`BackupConfigurationReady`'],
    },
    {
      cells: ['File audit storage', '`AuditFileStorageReady`, then `Degraded` if StatefulSet recreation is required'],
    },
    {
      cells: ['Restore execution', '`RestoreConfigurationReady`, then `RestoreComplete`'],
    },
  ]}
/>

## OpenBaoCluster conditions

Condition types defined in `api/v1alpha1`:

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `Available` | Workload availability from ready replicas | `AllReplicasReady`, `NoReplicasReady`, `NotReady`, `Paused` |
| `APIServerNetworkReady` | Operator-known Kubernetes API egress contract for operator-managed NetworkPolicies | `APIServerNetworkReady`, `APIServerEndpointIPsRecommended`, `APIServerNetworkConfigurationInvalid`, `Paused` |
| `TLSReady` | TLS asset readiness | `Ready`, `Disabled`, `TLSSecretMissing`, `TLSSecretInvalid`, `Unknown`, `Paused` |
| `UserAccessBootstrap` | Best-effort check that `spec.selfInit.requests` appears to create a human login path in addition to operator bootstrap auth | `UserAccessConfigured`, `UserAccessUnverified`, `Disabled`, `Paused` |
| `ACMEIntegrationReady` | Operator-known ACME prerequisites such as Gateway passthrough, private ACME trust, and supported self-reachability checks | `ACMEIntegrationReady`, `GatewayAPIMissing`, `ACMEGatewayNotConfiguredForPassthrough`, `ACMEDomainNotResolvable`, `PrerequisitesMissing`, `Unknown`, `Paused` |
| `ACMECacheReady` | Shared ACME cache readiness for HA or blue/green ACME topologies | `ACMECacheReady`, `ACMECacheNotConfigured`, `ACMECacheMissing`, `ACMECachePending`, `ACMECacheInvalidAccessMode` |
| `AuditFileStorageReady` | Shared file-audit PVC readiness and StatefulSet mount adoption | `AuditFileStorageReady`, `AuditFileStorageMissing`, `AuditFileStoragePending`, `AuditFileStorageInvalidAccessMode`, `AuditFileStorageStatefulSetRecreateRequired`, `Unknown` |
| `GatewayIntegrationReady` | Operator-known Gateway API prerequisites and controller support for `spec.gateway` | `GatewayIntegrationReady`, `GatewayAPIMissing`, `GatewayReferenceMissing`, `GatewayClassMissing`, `GatewayClassPending`, `GatewayClassNotAccepted`, `GatewayVersionUnsupported`, `GatewayFeatureUnsupported`, `GatewayCapabilitiesUnknown`, `GatewayNotProgrammed`, `GatewayProgrammingPending`, `GatewayListenerIncompatible`, `Paused` |
| `IngressIntegrationReady` | Operator-known managed Ingress prerequisites and controller support for `spec.ingress` | `IngressIntegrationReady`, `IngressClassMissing`, `IngressObjectPending`, `IngressLoadBalancerPending`, `IngressCapabilitiesUnknown`, `Unknown`, `Paused` |
| `BackupConfigurationReady` | Operator-known backup Job prerequisites such as auth references, storage credential references, hardened-profile egress rules, and job-specific identity assumptions | `Ready`, `AuthenticationRequired`, `TokenSecretMissing`, `CredentialsSecretMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `NetworkEgressRulesRequired`, `SecurityViolation`, `Unknown`, `Paused` |
| `CloudUnsealIdentityReady` | Operator-known authentication path for cloud KMS unseal on the main OpenBao Pods | `Ready`, `CredentialsSecretMissing`, `PrerequisitesMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `Unknown`, `Paused` |
| `ProductionReady` | Indicates whether the cluster currently meets the operator's Hardened production posture checks. This condition does not represent API stability or project support level. | `ProductionReady`, `ProfileNotSet`, `DevelopmentProfile`, `AdmissionPoliciesNotReady`, `UnsafeAdmissionDisabled`, `OperatorManagedTLS`, `StaticUnsealInUse`, `RootTokenStored`, `SecurityViolation`, Gateway or ACME readiness reasons such as `GatewayFeatureUnsupported` or `ACMEGatewayNotConfiguredForPassthrough` |
| `Upgrading` | Upgrade state | `InProgress`, `Idle`, or upgrade failure reason |
| `BackingUp` | Backup job state | `InProgress`, `Idle` |
| `StorageConfigured` | Persistent storage class selection visibility | `StorageClassConfigured`, `StorageClassPending`, `StorageClassDefaulted`, `StorageClassUnset`, `StorageClassMismatch`, `StorageClassInconsistent` |
| `ReadReplicasReady` | Read-replica Pod readiness against the declared pool size | `AllReadReplicasReady`, `NoReadReplicasConfigured`, `NoReadyReadReplicas`, `ReadReplicasNotReady`, `Unknown` |
| `ReadServingAvailable` | Whether an observed read replica is currently serving reads | `ReadServingAvailable`, `ReadServingWithoutQuorum`, `NoReadReplicasConfigured`, `NoReadyReadReplicas`, `PodsNotServingReads`, `Unknown` |
| `RaftMembershipReady` | Observed non-voter membership against the declared read-replica topology | `RaftMembershipReady`, `ReadReplicasNotReady`, `NoReadReplicasConfigured`, `Unknown` |
| `ReadReplicasAutopilotHealthy` | Raft Autopilot health for observed read-replica peers | `ReadReplicasAutopilotHealthy`, `ReadReplicasAutopilotUnhealthy`, `NoReadReplicasConfigured`, `Unknown` |
| `ReadReplicaStorageConfigured` | Effective storage class selection for read-replica PVCs | `StorageClassConfigured`, `StorageClassPending`, `StorageClassDefaulted`, `StorageClassUnset`, `StorageClassMismatch`, `StorageClassInconsistent`, `NoReadReplicasConfigured`, `Unknown` |
| `Degraded` | Problem requiring attention | `BreakGlassRequired`, upgrade failure reason, workload or adminops error reason, `OIDCBootstrapConfigurationInvalid`, `APIServerNetworkConfigurationInvalid`, `RootTokenStored`, `Reconciling`, `Paused` |
| `EtcdEncryptionWarning` | etcd encryption verification warning | `EtcdEncryptionUnknown` |
| `SecurityRisk` | Relaxed security mode indicator | `DevelopmentProfile`, `UnsafeAdmissionDisabled`, `SecurityViolation` |
| `OpenBaoInitialized` | OpenBao initialization observed from registration labels | `Initialized`, `NotInitialized`, `Unknown` |
| `OpenBaoSealed` | OpenBao seal state observed from registration labels | `Sealed`, `Unsealed`, `Unknown` |
| `OpenBaoLeader` | Leader discovery from registration labels | `LeaderFound`, `LeaderUnknown`, `MultipleLeaders` |
| `NodeSecurityCapabilityMismatch` | Node capability mismatch for enabled hardening | `Ready`, `AppArmorUnsupported` |

## OpenBaoRestore conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `RestoreComplete` | Restore terminal state | `RestoreSucceeded`, `RestoreFailed`, `AuthenticationRequired` |
| `RestoreConfigurationReady` | Operator-known restore prerequisites such as auth references, storage credential references, hardened-profile egress rules, and job-specific identity assumptions | `Ready`, `AuthenticationRequired`, `TokenSecretMissing`, `TokenSecretInvalid`, `CredentialsSecretMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `NetworkEgressRulesRequired`, `SecurityViolation` |
| `OperationLockOverride` | Break-glass lock override occurred | `OperationLockOverridden` |

<Callout type="note" title="Ambient identity reasons">

`AmbientIdentityAssumed` means the operator classified the configuration as relying on a provider default chain or other ambient identity path. It does not prove that the cloud-side identity binding is correct.

</Callout>

## OpenBaoTenant conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `Provisioned` | Tenant RBAC provisioning state | `SecurityViolation` and provisioning outcomes |

## Kubernetes events

<Callout type="note" title="Event scope">

The operator emits lifecycle events on parent custom resources only. `OpenBaoCluster` receives cluster lifecycle, init and bootstrap, upgrade, backup, and tenant Secret RBAC sync events. `OpenBaoRestore` receives restore lifecycle events. `OpenBaoTenant` receives tenant provisioning lifecycle events. Jobs do not receive the lifecycle events listed here.

</Callout>

<Callout type="tip" title="Event types">

Expect `Normal` events for routine progression and accepted operator input. Expect `Warning` events for failures, contention, overrides, and other states that need attention.

</Callout>

### OpenBaoCluster safety and maintenance events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Warning` | `ProfileNotSet` | `spec.profile` missing; reconciliation blocked. |
| `Warning` | `DevelopmentProfile` | Development profile warning for production. |
| `Warning` | `UnsafeAdmissionDisabled` | Unsafe admission mode is active, required admission guardrails are not enforced, and Hardened clusters are not production-ready. |
| `Normal` | `AmbientUnsealIdentity` | Cloud KMS unseal is relying on ambient identity or the provider default chain for the main OpenBao Pods. This note is emitted only when the operator is not using a credentials Secret or explicit inline cloud credentials. |
| `Warning` | `StaticUnsealInUse` | Static unseal warning. |
| `Warning` | `RootTokenStored` | Self-init is disabled and the operator stored the root token Secret. |
| `Warning` | `ImageVerificationFailed` and related reasons | Warn-policy image verification failures. |
| `Normal` | `PVCResize` | PVC expansion started. |
| `Normal` | `PVCResizeLeaderStepDown` | Leader step-down for resize restart path. |
| `Normal` | `PVCResizePodRestart` | Pod restart to complete filesystem resize. |

### OpenBaoCluster init and bootstrap events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `InitStarted` | Self-init or operator-driven initialization started or is still in progress. |
| `Normal` | `InitCompleted` | Cluster initialization completed successfully. |
| `Warning` | `InitFailed` | Operator-driven initialization failed. |

### OpenBaoCluster tenant Secret RBAC events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `TenantSecretRBACSynchronized` | Tenant Secret RBAC allowlists were synchronized for the namespace. |

### OpenBaoCluster upgrade events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `UpgradeStarted` | Upgrade orchestration started. |
| `Normal` | `PreUpgradeSnapshotJobCreated` | Pre-upgrade snapshot Job created. |
| `Normal` | `PreUpgradeSnapshotCompleted` | Pre-upgrade snapshot completed successfully. |
| `Warning` | `PreUpgradeSnapshotFailed` | Pre-upgrade snapshot failed and upgrade is blocked. |
| `Normal` | `RollingRetryRequested` | Manual retry requested for a failed rolling upgrade. |
| `Normal` | `RollingRetryAccepted` | Failed rolling upgrade state cleared and retry resumed. |
| `Normal` | `BlueGreenHoldEntered` | Blue or green upgrade is waiting for manual promotion approval. |
| `Normal` | `BlueGreenPromotionApproved` | Promotion approval observed and promotion started. |
| `Normal` | `UpgradeComplete` | Upgrade finished successfully. |
| `Warning` | `UpgradeFailed` | Upgrade failed and operator marked the upgrade as failed. |
| `Warning` | `RollbackStarted` | Blue or green rollback started. |
| `Warning` | `BreakGlassEntered` | Blue or green rollback entered break-glass mode. |
| `Normal` | `BreakGlassAcknowledged` | Break-glass mode was acknowledged and automation may resume. |
| `Warning` | `OperationLockBlocked` | Upgrade is waiting for another cluster operation to release the lock. |

### OpenBaoCluster backup events

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

### OpenBaoRestore events

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

### OpenBaoTenant provisioning events

| Type | Reason | Notes |
| :--- | :--- | :--- |
| `Normal` | `TenantProvisioned` | Tenant namespace RBAC was provisioned successfully. |
| `Normal` | `TenantRBACCleaned` | Tenant namespace RBAC was cleaned up during deletion. |
| `Warning` | `TenantProvisioningBlocked` | Provisioning is blocked by guardrails, missing prerequisites, or dependency readiness checks. |
| `Warning` | `TenantProvisioningFailed` | Provisioning failed while applying tenant RBAC. |

## Structured audit events

In addition to Kubernetes Events, controllers emit structured audit events to logs, for example `UpgradeStarted`, `UpgradeFailed`, `BackupJobCreated`, `RestoreCompleted`, and `TenantRBACProvisioned`.

<Callout type="note" title="Stability">

Condition **types** are part of the API surface. Reason and event values may expand over time as new scenarios are added.

</Callout>

<NextActions
  title="Related lookup surfaces"
  items={[
    {
      label: 'Unseal configuration',
      description: 'Provider and Secret requirements behind `CloudUnsealIdentityReady` and seal-mode setup.',
      docId: 'user-guide/openbaocluster/configuration/unseal',
    },
    {
      label: 'Troubleshoot the cluster',
      description: 'Operational troubleshooting when the relevant signal is not yet clear.',
      to: '/docs/operate/troubleshooting',
    },
    {
      label: 'Recovery & Restore',
      description: 'Move into the recovery section when the condition or event pattern already tells you the system needs intervention.',
      to: '/docs/recover',
    },
    {
      label: 'Compatibility matrix',
      description: 'Return to compatibility when an unexpected status might actually come from an unsupported or unvalidated platform assumption.',
      docId: 'reference/compatibility',
    },
  ]}
/>
