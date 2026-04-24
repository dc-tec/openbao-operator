/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// OpenBaoClusterClaimFinalizer protects claim deletion until concrete same-cluster materialization has been cleaned up.
	OpenBaoClusterClaimFinalizer = "claims.openbao.org/claim-protection"
)

// OpenBaoClusterClaimPhase summarizes user-facing claim state.
// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Degraded;Failed;Deleting
type OpenBaoClusterClaimPhase string

const (
	// OpenBaoClusterClaimPhasePending indicates the claim has been accepted but not yet materialized.
	OpenBaoClusterClaimPhasePending OpenBaoClusterClaimPhase = "Pending"
	// OpenBaoClusterClaimPhaseProvisioning indicates same-cluster materialization is in progress.
	OpenBaoClusterClaimPhaseProvisioning OpenBaoClusterClaimPhase = "Provisioning"
	// OpenBaoClusterClaimPhaseReady indicates the claim is ready for use.
	OpenBaoClusterClaimPhaseReady OpenBaoClusterClaimPhase = "Ready"
	// OpenBaoClusterClaimPhaseDegraded indicates the claim is available but degraded.
	OpenBaoClusterClaimPhaseDegraded OpenBaoClusterClaimPhase = "Degraded"
	// OpenBaoClusterClaimPhaseFailed indicates the claim cannot progress safely.
	OpenBaoClusterClaimPhaseFailed OpenBaoClusterClaimPhase = "Failed"
	// OpenBaoClusterClaimPhaseDeleting indicates teardown is in progress.
	OpenBaoClusterClaimPhaseDeleting OpenBaoClusterClaimPhase = "Deleting"
)

// OpenBaoClusterClaimBackupServiceParametersSpec defines the bounded claim-facing
// backup override surface.
type OpenBaoClusterClaimBackupServiceParametersSpec struct {
	// Location requests an allowed backup location value when the resolved target permits it.
	// +optional
	Location string `json:"location,omitempty"`
	// Partition requests an allowed backup partition value when the resolved target permits it.
	// +optional
	Partition string `json:"partition,omitempty"`
}

// OpenBaoClusterClaimServiceParametersSpec defines the bounded claim-facing
// parameter surface.
type OpenBaoClusterClaimServiceParametersSpec struct {
	// Backup carries the bounded backup override surface.
	// +optional
	Backup *OpenBaoClusterClaimBackupServiceParametersSpec `json:"backup,omitempty"`
	// Exposure carries the bounded exposure override surface.
	// +optional
	Exposure *OpenBaoClusterClaimExposureServiceParametersSpec `json:"exposure,omitempty"`
}

// OpenBaoClusterClaimExposureServiceParametersSpec defines bounded claim-facing
// exposure parameters.
type OpenBaoClusterClaimExposureServiceParametersSpec struct {
	// Hostname requests a hostname when the selected exposure class allows
	// tenant-provided hostnames.
	// +optional
	Hostname string `json:"hostname,omitempty"`
}

// OpenBaoClusterClaimSpec defines the desired state of OpenBaoClusterClaim.
type OpenBaoClusterClaimSpec struct {
	// TenantRef identifies the tenant that governs this claim.
	TenantRef LocalReference `json:"tenantRef"`
	// ServiceOfferingRef identifies the friendly stable service-offering alias selected by the claim.
	// +optional
	ServiceOfferingRef *LocalReference `json:"serviceOfferingRef,omitempty"`
	// ServiceProfileRef identifies the pinned immutable service-offering revision requested by the claim.
	ServiceProfileRef LocalReference `json:"serviceProfileRef"`
	// ServiceParameters carries the bounded claim-facing override surface.
	// +optional
	ServiceParameters *OpenBaoClusterClaimServiceParametersSpec `json:"serviceParameters,omitempty"`
}

// OpenBaoClusterClaimConnectionStatus is the narrow user-facing connection summary.
type OpenBaoClusterClaimConnectionStatus struct {
	// Endpoint is the published user-facing endpoint when available.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// CABundleRef identifies connection CA material when it has been published.
	// +optional
	CABundleRef *TypedObjectReference `json:"caBundleRef,omitempty"`
	// SecretRef identifies the claim-owned connection Secret when it has been published.
	// +optional
	SecretRef *LocalReference `json:"secretRef,omitempty"`
	// ObservedAt is when the connection contract was last observed and published.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`
}

// OpenBaoClusterClaimMaterializationMode identifies the materialization path selected for the claim.
// +kubebuilder:validation:Enum=SameCluster
type OpenBaoClusterClaimMaterializationMode string

const (
	// OpenBaoClusterClaimMaterializationModeSameCluster indicates the claim will materialize locally into OpenBaoCluster.
	OpenBaoClusterClaimMaterializationModeSameCluster OpenBaoClusterClaimMaterializationMode = "SameCluster"
)

// OpenBaoClusterClaimMaterializationStatus summarizes the currently selected materialization path.
type OpenBaoClusterClaimMaterializationStatus struct {
	// Mode identifies the materialization path currently selected for the claim.
	// +optional
	Mode OpenBaoClusterClaimMaterializationMode `json:"mode,omitempty"`
	// LocalRef identifies the intended same-cluster concrete workload object when materialization is local.
	// +optional
	LocalRef *NamespacedReference `json:"localRef,omitempty"`
}

// OpenBaoClusterClaimBoundRevisionReference identifies a bound immutable catalog revision by name and UID.
type OpenBaoClusterClaimBoundRevisionReference struct {
	// Name is the immutable catalog object name.
	Name string `json:"name"`
	// UID is the Kubernetes object UID observed when the claim bound this revision.
	UID string `json:"uid"`
}

// OpenBaoClusterClaimContractIdentityStatus identifies one internal contract revision by stable hash.
type OpenBaoClusterClaimContractIdentityStatus struct {
	// IdentityHash is the stable content hash for the applied internal contract revision.
	IdentityHash string `json:"identityHash"`
}

// OpenBaoClusterClaimRenderedDependencyStatus summarizes the currently applied
// lower execution-policy dependency identities for same-cluster rendering.
type OpenBaoClusterClaimRenderedDependencyStatus struct {
	// EntrypointRef identifies the currently applied immutable entrypoint revision.
	// +optional
	EntrypointRef *OpenBaoClusterClaimBoundRevisionReference `json:"entrypointRef,omitempty"`
	// IngressPolicyRef identifies the currently applied immutable ingress-policy revision.
	// +optional
	IngressPolicyRef *OpenBaoClusterClaimBoundRevisionReference `json:"ingressPolicyRef,omitempty"`
	// BackupTargetRef identifies the currently applied immutable backup-target revision.
	// +optional
	BackupTargetRef *OpenBaoClusterClaimBoundRevisionReference `json:"backupTargetRef,omitempty"`
	// BackupBackendRef identifies the currently applied immutable backup-backend revision.
	// +optional
	BackupBackendRef *OpenBaoClusterClaimBoundRevisionReference `json:"backupBackendRef,omitempty"`
	// BackupAuthProfileRef identifies the currently applied immutable backup-auth-profile revision.
	// +optional
	BackupAuthProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"backupAuthProfileRef,omitempty"`
	// TransferProfileRef identifies the currently applied immutable transfer-profile revision.
	// +optional
	TransferProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"transferProfileRef,omitempty"`
	// BootstrapProjectionIdentity identifies the currently applied projected
	// bootstrap dependency artifact set for same-cluster execution.
	// +optional
	BootstrapProjectionIdentity *OpenBaoClusterClaimContractIdentityStatus `json:"bootstrapProjectionIdentity,omitempty"`
	// BootstrapProjectionRefs identifies the concrete projected bootstrap
	// artifacts currently consumed by same-cluster execution.
	// +optional
	BootstrapProjectionRefs []TypedObjectReference `json:"bootstrapProjectionRefs,omitempty"`
	// Identity identifies the currently applied rendered dependency revision set.
	// +optional
	Identity *OpenBaoClusterClaimContractIdentityStatus `json:"identity,omitempty"`
}

// OpenBaoClusterClaimAppliedStatus summarizes the currently applied bound revision identities.
type OpenBaoClusterClaimAppliedStatus struct {
	// ServiceOfferingRef identifies the currently applied stable service-offering alias when the claim was bound through one.
	// +optional
	ServiceOfferingRef *LocalReference `json:"serviceOfferingRef,omitempty"`
	// ServiceProfileRef identifies the currently applied immutable service-profile revision.
	// +optional
	ServiceProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"serviceProfileRef,omitempty"`
	// BootstrapProfileRef identifies the currently applied immutable bootstrap-profile revision.
	// +optional
	BootstrapProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"bootstrapProfileRef,omitempty"`
	// ExposureClassRef identifies the currently applied immutable exposure-class revision.
	// +optional
	ExposureClassRef *OpenBaoClusterClaimBoundRevisionReference `json:"exposureClassRef,omitempty"`
	// StorageProfileRef identifies the currently applied immutable storage-profile revision.
	// +optional
	StorageProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"storageProfileRef,omitempty"`
	// UnsealProfileRef identifies the currently applied immutable unseal-profile revision.
	// +optional
	UnsealProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"unsealProfileRef,omitempty"`
	// RuntimeProfileRef identifies the currently applied immutable runtime-profile revision.
	// +optional
	RuntimeProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"runtimeProfileRef,omitempty"`
	// ObservabilityProfileRef identifies the currently applied immutable observability-profile revision.
	// +optional
	ObservabilityProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"observabilityProfileRef,omitempty"`
	// NetworkProfileRef identifies the currently applied immutable network-profile revision.
	// +optional
	NetworkProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"networkProfileRef,omitempty"`
	// UpgradePolicyRef identifies the currently applied immutable upgrade-policy revision.
	// +optional
	UpgradePolicyRef *OpenBaoClusterClaimBoundRevisionReference `json:"upgradePolicyRef,omitempty"`
	// BackupProfileRef identifies the currently applied immutable backup-profile revision.
	// +optional
	BackupProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"backupProfileRef,omitempty"`
	// ApprovedContract identifies the currently applied approved-service contract revision.
	// +optional
	ApprovedContract *OpenBaoClusterClaimContractIdentityStatus `json:"approvedContract,omitempty"`
	// RenderedContract identifies the currently applied rendered-execution contract revision.
	// +optional
	RenderedContract *OpenBaoClusterClaimContractIdentityStatus `json:"renderedContract,omitempty"`
	// RenderedDependencies identifies the currently applied lower execution-policy dependency revisions.
	// +optional
	RenderedDependencies *OpenBaoClusterClaimRenderedDependencyStatus `json:"renderedDependencies,omitempty"`
}

// OpenBaoClusterClaimRolloutState summarizes rollout progress for a materialized claim.
// +kubebuilder:validation:Enum=Idle;Pending;Rendering;RollingOut;Blocked;Failed
type OpenBaoClusterClaimRolloutState string

const (
	// OpenBaoClusterClaimRolloutStateIdle indicates there is no rollout in progress.
	OpenBaoClusterClaimRolloutStateIdle OpenBaoClusterClaimRolloutState = "Idle"
	// OpenBaoClusterClaimRolloutStatePending indicates rollout evaluation has not completed yet.
	OpenBaoClusterClaimRolloutStatePending OpenBaoClusterClaimRolloutState = "Pending"
	// OpenBaoClusterClaimRolloutStateRendering indicates internal contract rendering is in progress.
	OpenBaoClusterClaimRolloutStateRendering OpenBaoClusterClaimRolloutState = "Rendering"
	// OpenBaoClusterClaimRolloutStateRollingOut indicates the system is converging to a new rendered contract.
	OpenBaoClusterClaimRolloutStateRollingOut OpenBaoClusterClaimRolloutState = "RollingOut"
	// OpenBaoClusterClaimRolloutStateBlocked indicates the requested change is blocked.
	OpenBaoClusterClaimRolloutStateBlocked OpenBaoClusterClaimRolloutState = "Blocked"
	// OpenBaoClusterClaimRolloutStateFailed indicates rollout was supported but did not converge successfully.
	OpenBaoClusterClaimRolloutStateFailed OpenBaoClusterClaimRolloutState = "Failed"
)

// OpenBaoClusterClaimRolloutStatus summarizes rollout state for the claim.
type OpenBaoClusterClaimRolloutStatus struct {
	// State is the current rollout state.
	// +optional
	State OpenBaoClusterClaimRolloutState `json:"state,omitempty"`
	// Reason provides the current rollout-state reason.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// OpenBaoClusterClaimUpgradeStatus summarizes the currently active
// claim-upgrade workflow when one exists.
type OpenBaoClusterClaimUpgradeStatus struct {
	// RequestRef identifies the active immutable upgrade-request object.
	// +optional
	RequestRef *LocalReference `json:"requestRef,omitempty"`
	// State is the current workflow state reported by the upgrade request.
	// +optional
	State OpenBaoClusterClaimUpgradeRequestState `json:"state,omitempty"`
	// Reason explains the current workflow state.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Classification summarizes the evaluated compatibility class when available.
	// +optional
	Classification *OpenBaoClusterClaimUpgradeRequestClassificationStatus `json:"classification,omitempty"`
}

// OpenBaoClusterClaimRestoreStatus summarizes the currently active
// restore workflow when one exists for the service instance.
type OpenBaoClusterClaimRestoreStatus struct {
	// RequestRef identifies the active immutable claim-restore-request object when one exists.
	// +optional
	RequestRef *LocalReference `json:"requestRef,omitempty"`
	// RequestState is the current workflow state reported by the claim restore request.
	// +optional
	RequestState OpenBaoClusterClaimRestoreRequestState `json:"requestState,omitempty"`
	// RequestReason explains the current request workflow state.
	// +optional
	RequestReason string `json:"requestReason,omitempty"`
	// ExecutionRef identifies the active underlying OpenBaoRestore execution object when one exists.
	// +optional
	ExecutionRef *NamespacedReference `json:"executionRef,omitempty"`
	// State is the current workflow phase reported by the underlying restore execution.
	// +optional
	State RestorePhase `json:"state,omitempty"`
	// SnapshotKey identifies the snapshot currently being restored when available.
	// +optional
	SnapshotKey string `json:"snapshotKey,omitempty"`
	// StartTime is when the restore workflow started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// Message is the current best-effort restore workflow message.
	// +optional
	Message string `json:"message,omitempty"`
}

// OpenBaoClusterClaimBackupStatus summarizes backup lifecycle state for the service instance.
type OpenBaoClusterClaimBackupStatus struct {
	// RequestRef identifies the active immutable backup-request object.
	// +optional
	RequestRef *LocalReference `json:"requestRef,omitempty"`
	// RequestState is the current workflow state reported by the backup request.
	// +optional
	RequestState OpenBaoClusterClaimBackupRequestState `json:"requestState,omitempty"`
	// RequestReason explains the current workflow state.
	// +optional
	RequestReason string `json:"requestReason,omitempty"`
	// InProgress indicates whether a backup operation is currently active.
	// +optional
	InProgress bool `json:"inProgress,omitempty"`
	// LastBackupTime is the timestamp of the last successful backup.
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
	// LastBackupName is the object key/path of the last successful backup.
	// +optional
	LastBackupName string `json:"lastBackupName,omitempty"`
	// LastAttemptTime is the timestamp of the last backup attempt, regardless of outcome.
	// +optional
	LastAttemptTime *metav1.Time `json:"lastAttemptTime,omitempty"`
	// NextScheduledBackup is when the next backup is scheduled.
	// +optional
	NextScheduledBackup *metav1.Time `json:"nextScheduledBackup,omitempty"`
	// LastBackupDuration is how long the last successful backup took.
	// +optional
	LastBackupDuration string `json:"lastBackupDuration,omitempty"`
	// ConsecutiveFailures is the number of consecutive backup failures.
	// +optional
	ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`
	// LastFailureReason is the low-cardinality reason for the last backup failure.
	// +optional
	LastFailureReason string `json:"lastFailureReason,omitempty"`
	// LastFailureMessage is the detailed message for the last backup failure.
	// +optional
	LastFailureMessage string `json:"lastFailureMessage,omitempty"`
}

// OpenBaoClusterClaimStatusSeverity classifies the current claim summary.
// +kubebuilder:validation:Enum=Info;Warning;Error
type OpenBaoClusterClaimStatusSeverity string

const (
	// OpenBaoClusterClaimStatusSeverityInfo indicates an informational current state.
	OpenBaoClusterClaimStatusSeverityInfo OpenBaoClusterClaimStatusSeverity = "Info"
	// OpenBaoClusterClaimStatusSeverityWarning indicates a current state requiring attention but not a hard failure.
	OpenBaoClusterClaimStatusSeverityWarning OpenBaoClusterClaimStatusSeverity = "Warning"
	// OpenBaoClusterClaimStatusSeverityError indicates a hard failure or invalid state.
	OpenBaoClusterClaimStatusSeverityError OpenBaoClusterClaimStatusSeverity = "Error"
)

// OpenBaoClusterClaimStatusSummary provides a single current diagnostic summary
// for the service instance when it is not in a steady ready state.
type OpenBaoClusterClaimStatusSummary struct {
	// Severity classifies the current summary.
	// +optional
	Severity OpenBaoClusterClaimStatusSeverity `json:"severity,omitempty"`
	// Reason is a low-cardinality reason for the current summary.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is the current best-effort human-readable summary.
	// +optional
	Message string `json:"message,omitempty"`
	// SourceRef identifies the object currently driving this summary when available.
	// +optional
	SourceRef *TypedObjectReference `json:"sourceRef,omitempty"`
}

// OpenBaoClusterClaimStatus defines the observed state of OpenBaoClusterClaim.
type OpenBaoClusterClaimStatus struct {
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Phase is the user-facing claim phase.
	// +optional
	Phase OpenBaoClusterClaimPhase `json:"phase,omitempty"`
	// Materialization summarizes the currently selected materialization path.
	// +optional
	Materialization OpenBaoClusterClaimMaterializationStatus `json:"materialization,omitempty"`
	// Applied summarizes the currently applied immutable revision identities.
	// +optional
	Applied OpenBaoClusterClaimAppliedStatus `json:"applied,omitempty"`
	// Rollout summarizes rollout progress for the claim.
	// +optional
	Rollout OpenBaoClusterClaimRolloutStatus `json:"rollout,omitempty"`
	// Upgrade summarizes the currently active upgrade workflow when one exists.
	// +optional
	Upgrade *OpenBaoClusterClaimUpgradeStatus `json:"upgrade,omitempty"`
	// Restore summarizes the currently active restore workflow when one exists.
	// +optional
	Restore *OpenBaoClusterClaimRestoreStatus `json:"restore,omitempty"`
	// Backup summarizes claim-facing backup lifecycle state when available.
	// +optional
	Backup *OpenBaoClusterClaimBackupStatus `json:"backup,omitempty"`
	// Summary is the current best-effort diagnostic summary when the claim is not in a steady ready state.
	// +optional
	Summary *OpenBaoClusterClaimStatusSummary `json:"summary,omitempty"`
	// Connection summarizes published connection details.
	// +optional
	Connection OpenBaoClusterClaimConnectionStatus `json:"connection,omitempty"`
	// Conditions represent the latest available observations of the claim state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Tenant",type="string",JSONPath=".spec.tenantRef.name"
// +kubebuilder:printcolumn:name="Offering",type="string",JSONPath=".spec.serviceOfferingRef.name"
// +kubebuilder:printcolumn:name="Profile",type="string",JSONPath=".spec.serviceProfileRef.name"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoClusterClaim is the user-facing namespaced request for an OpenBao service instance.
type OpenBaoClusterClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoClusterClaimSpec   `json:"spec"`
	Status OpenBaoClusterClaimStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoClusterClaimList contains a list of OpenBaoClusterClaim.
type OpenBaoClusterClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoClusterClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoClusterClaim{}, &OpenBaoClusterClaimList{})
}
