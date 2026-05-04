---
title: Status Conditions and Events
description: Reference for OpenBao Operator status conditions, workflow states, Kubernetes Events, and audit events for cluster, claim, restore, and tenant resources.
pageType: reference
journey: reference
---

<PageHeader
  title="Status conditions and event reference"
  lede="Status conditions, workflow states, and emitted events across `OpenBaoCluster`, `OpenBaoClusterClaim`, claim workflow requests, `OpenBaoRestore`, and `OpenBaoTenant`."
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect status conditions and namespace events"
  code={`kubectl -n <ns> get openbaoclusterclaim <name> -o yaml
kubectl -n <ns> get openbaoclusterclaimupgraderequest <name> -o yaml
kubectl -n <ns> get openbaoclusterclaimbackuprequest <name> -o yaml
kubectl -n <ns> get openbaoclusterclaimrestorerequest <name> -o yaml
kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaorestore <name> -o jsonpath='{.status.conditions}' | jq
kubectl -n <ns> get openbaotenant <name> -o jsonpath='{.status.conditions}' | jq

kubectl -n <ns> get events --sort-by=.lastTimestamp`}
>
  For claims, start with the full object instead of just `.status.conditions`. The claim phase, summary, workflow sub-status, and applied revision data carry more signal than conditions alone.
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
    {
      cells: ['Service claims', '`Accepted`, `ServiceContractReady`, `MaterializationResolved`, `OwnershipReady`, `ConnectionPublished`, `ServiceAvailable`'],
    },
    {
      cells: ['Claim maintenance workflows', '`MaintenanceActive`, plus `status.upgrade`, `status.restore`, or `status.backup`'],
    },
  ]}
/>

## OpenBaoClusterClaim status

`OpenBaoClusterClaim` uses a phase plus several focused sub-status surfaces:

| Field | Meaning |
| :--- | :--- |
| `status.phase` | User-facing claim lifecycle state: `Pending`, `Provisioning`, `Ready`, `Degraded`, `Failed`, `Deleting` |
| `status.materialization` | Whether the service is materialized through the supported same-cluster path, plus the current local reference |
| `status.applied` | The applied service-offering alias, immutable service-profile revision, and rendered contract identities |
| `status.rollout` | Claim rollout state for materialized revisions |
| `status.connection` | Published endpoint, CA bundle reference, connection Secret reference, and `observedAt` |
| `status.upgrade` | Active claim upgrade workflow summary when an `OpenBaoClusterClaimUpgradeRequest` is in progress |
| `status.restore` | Active claim restore workflow summary, including the request object and the underlying `OpenBaoRestore` execution when one exists |
| `status.backup` | Backup history plus active manual backup request state |
| `status.summary` | The current best-effort diagnostic summary, including severity, reason, message, and source object |

### OpenBaoClusterClaim conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `ControllerActive` | Claim controller availability behind the feature gate | `NotImplemented`, `FeatureDisabled` |
| `Accepted` | Tenant and platform governance accepted the claim | `Accepted`, `Pending`, `Invalid`, `FeatureDisabled` |
| `ServiceContractReady` | Immutable service contract resolved from catalog inputs | `Accepted`, `Pending`, `Invalid`, `FeatureDisabled` |
| `MaterializationResolved` | Concrete same-cluster materialization path resolved | `Accepted`, `Pending`, `PlacementPending`, `Invalid`, `FeatureDisabled` |
| `OwnershipReady` | Same-cluster custody boundary is safe | `Accepted`, `Invalid` |
| `ConnectionPublished` | Connection Secret and endpoint publication contract is valid | `Ready`, `Pending`, `Invalid` |
| `ServiceAvailable` | Whether the service instance is currently usable | `Ready`, `Pending`, `Invalid`, `Deleting`, `BackingUp`, backup failure reasons such as `BackupScheduleFailed`, active upgrade-request states such as `RollingOut`, active restore phases such as `Running` |
| `MaintenanceActive` | Whether a maintenance workflow is acting on the service instance | `Idle`, active upgrade-request states such as `RollingOut`, active restore phases such as `Running` |

### OpenBaoClusterClaim summary severities

| Severity | Meaning |
| :--- | :--- |
| `Info` | Current non-terminal workflow or provisioning state |
| `Warning` | Service is still available, but backup or restore work needs attention |
| `Error` | Invalid or failed claim state that requires operator action |

## Claim workflow request states

The claim workflow request objects are immutable, namespaced request records. The operator currently uses `status.state` and `status.reason` as the primary lifecycle surface for these objects. `status.conditions` is reserved for later expansion and is not populated today.

### OpenBaoClusterClaimUpgradeRequest states

| State | Meaning | Common Reasons |
| :--- | :--- | :--- |
| `Pending` | Request admitted and waiting for target resolution or claim promotion | initial admission before target promotion |
| `RollingOut` | Claim target was promoted and the in-place rollout is converging | `RolloutRequested`, `AppliedRevisionPending`, `ClaimRolloutInProgress`, `UpgradeInProgress`, `LocalClusterReconciling`, `ClaimNotReadyYet` |
| `Succeeded` | In-place upgrade completed successfully | `UpgradeApplied` |
| `Blocked` | Request is outside the supported in-place claim upgrade boundary | `ServiceClaimsDisabled`, `AnotherUpgradeRequestActive`, `ClaimDeleting`, `ClaimNotMaterializedForSameCluster`, `ClaimHasNoAppliedRevision`, `AlreadyApplied`, blocked classification reasons such as `BootstrapChangeRequiresReprovision`, `BackupLocationChangeUnsupported`, or `UnsupportedServiceShapeChange` |
| `Failed` | Request could not be evaluated or could not complete rollout safely | `ClaimNotFound`, `ClaimReadFailed`, `CurrentCatalogResolutionFailed`, `TargetCatalogResolutionFailed`, `ClaimUpdateFailed`, `ClaimRolloutBlocked`, `ClaimRolloutFailed`, `LocalClusterFailed` |

### OpenBaoClusterClaimBackupRequest states

| State | Meaning | Common Reasons |
| :--- | :--- | :--- |
| `Pending` | Manual backup request admitted and trigger annotation written | `BackupRequested` |
| `Running` | The backup attempt is active or waiting for terminal observation | `BackupInProgress`, `BackupCompletionPending` |
| `Succeeded` | The backup request completed successfully | `BackupCompleted` |
| `Blocked` | Request is outside the supported same-cluster manual backup model | `ServiceClaimsDisabled`, `AnotherBackupRequestActive`, `ClaimDeleting`, `ClaimNotMaterializedForSameCluster`, `LocalClusterDeleting` |
| `Failed` | Request could not be observed or the backup attempt failed | `ClaimNotFound`, `ClaimReadFailed`, `BackupRequestListFailed`, `LocalClusterNotFound`, `LocalClusterReadFailed`, `TriggerUpdateFailed`, backup failure reasons such as `BackupFailed` |

### OpenBaoClusterClaimRestoreRequest states

| State | Meaning | Common Reasons |
| :--- | :--- | :--- |
| `Pending` | Restore request admitted and the underlying restore execution is being created or validated | `RestoreRequested`, `Pending`, `Validating` |
| `Running` | The underlying restore execution is actively validating or restoring data | `Validating`, `Running` |
| `Succeeded` | The restore request completed successfully | `RestoreCompleted` |
| `Blocked` | Request is outside the supported same-cluster restore model | `ServiceClaimsDisabled`, `AnotherRestoreRequestActive`, `ClaimDeleting`, `ClaimNotMaterializedForSameCluster`, `LocalClusterDeleting`, `BackupNotConfigured`, `NoSuccessfulBackupAvailable`, `InvalidRestoreSource`, `BackupRequestRefRequired`, `BackupRequestNotFound`, `BackupRequestClaimMismatch`, `BackupRequestClusterUnknown`, `BackupRequestClusterMismatch`, `BackupRequestNotSucceeded`, `BackupRequestSnapshotMissing`, `AnotherRestoreExecutionActive`, `RestoreExecutionNameConflict` |
| `Failed` | Request could not be observed or the underlying restore execution failed | `ClaimNotFound`, `ClaimReadFailed`, `RestoreRequestListFailed`, `LocalClusterNotFound`, `LocalClusterReadFailed`, `BackupRequestReadFailed`, `RestoreExecutionListFailed`, `RestoreExecutionReadFailed`, `RestoreCreateFailed`, restore failure reasons such as `RestoreFailed` |

<Callout type="note" title="Claims are status-first today">

`OpenBaoClusterClaim`, `OpenBaoClusterClaimUpgradeRequest`, `OpenBaoClusterClaimBackupRequest`, and `OpenBaoClusterClaimRestoreRequest` currently rely on status, summary, and workflow-state fields as the primary operational timeline. They do not emit the same lifecycle Event surface that `OpenBaoCluster`, `OpenBaoRestore`, and `OpenBaoTenant` do today.

</Callout>

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
| `BackupConfigurationReady` | Operator-known backup Job prerequisites such as auth references, storage credential references, hardened-profile egress rules, and job-specific identity assumptions | `Ready`, `AuthenticationRequired`, `TokenSecretMissing`, `CredentialsSecretMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `NetworkEgressRulesRequired`, `Unknown`, `Paused` |
| `CloudUnsealIdentityReady` | Operator-known authentication path for cloud KMS unseal on the main OpenBao Pods | `Ready`, `CredentialsSecretMissing`, `PrerequisitesMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `Unknown`, `Paused` |
| `ProductionReady` | Indicates whether the cluster currently meets the operator's Hardened production posture checks. This condition does not represent API stability or project support level. | `ProductionReady`, `ProfileNotSet`, `DevelopmentProfile`, `AdmissionPoliciesNotReady`, `UnsafeAdmissionDisabled`, `OperatorManagedTLS`, `StaticUnsealInUse`, `RootTokenStored`, Gateway or ACME readiness reasons such as `GatewayFeatureUnsupported` or `ACMEGatewayNotConfiguredForPassthrough` |
| `Upgrading` | Upgrade state | `InProgress`, `Idle`, or upgrade failure reason |
| `BackingUp` | Backup job state | `InProgress`, `Idle` |
| `StorageConfigured` | Persistent storage class selection visibility | `StorageClassConfigured`, `StorageClassPending`, `StorageClassDefaulted`, `StorageClassUnset`, `StorageClassMismatch`, `StorageClassInconsistent` |
| `Degraded` | Problem requiring attention | `BreakGlassRequired`, upgrade failure reason, workload or adminops error reason, `OIDCBootstrapConfigurationInvalid`, `APIServerNetworkConfigurationInvalid`, `RootTokenStored`, `Reconciling`, `Paused` |
| `EtcdEncryptionWarning` | etcd encryption verification warning | `EtcdEncryptionUnknown` |
| `SecurityRisk` | Relaxed security mode indicator | `DevelopmentProfile`, `UnsafeAdmissionDisabled` |
| `OpenBaoInitialized` | OpenBao initialization observed from registration labels | `Initialized`, `NotInitialized`, `Unknown` |
| `OpenBaoSealed` | OpenBao seal state observed from registration labels | `Sealed`, `Unsealed`, `Unknown` |
| `OpenBaoLeader` | Leader discovery from registration labels | `LeaderFound`, `LeaderUnknown`, `MultipleLeaders` |
| `NodeSecurityCapabilityMismatch` | Node capability mismatch for enabled hardening | `Ready`, `AppArmorUnsupported` |

## OpenBaoRestore conditions

| Type | Meaning | Typical Reasons |
| :--- | :--- | :--- |
| `RestoreComplete` | Restore terminal state | `RestoreSucceeded`, `RestoreFailed`, `AuthenticationRequired` |
| `RestoreConfigurationReady` | Operator-known restore prerequisites such as auth references, storage credential references, hardened-profile egress rules, and job-specific identity assumptions | `Ready`, `AuthenticationRequired`, `TokenSecretMissing`, `TokenSecretInvalid`, `CredentialsSecretMissing`, `WorkloadIdentityConfigured`, `AmbientIdentityAssumed`, `NetworkEgressRulesRequired` |
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

The operator emits lifecycle events on parent custom resources only. `OpenBaoCluster` receives cluster lifecycle, init and bootstrap, upgrade, backup, and tenant Secret RBAC sync events. `OpenBaoRestore` receives restore lifecycle events. `OpenBaoTenant` receives tenant provisioning lifecycle events. Claim and claim-workflow objects are currently status-driven and do not emit a dedicated lifecycle Event set. Jobs do not receive the lifecycle events listed here.

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
      label: 'Service claims troubleshooting',
      description: 'Use the claim troubleshooting flow when the claim phase or summary is the first failing surface.',
      docId: 'user-guide/service-claims/troubleshooting',
    },
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
