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

// OpenBaoClusterClaimRestoreRequestState summarizes request progress.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Blocked;Failed
type OpenBaoClusterClaimRestoreRequestState string

const (
	// OpenBaoClusterClaimRestoreRequestStatePending indicates the request was admitted but execution has not completed yet.
	OpenBaoClusterClaimRestoreRequestStatePending OpenBaoClusterClaimRestoreRequestState = "Pending"
	// OpenBaoClusterClaimRestoreRequestStateRunning indicates the restore workflow is actively converging.
	OpenBaoClusterClaimRestoreRequestStateRunning OpenBaoClusterClaimRestoreRequestState = "Running"
	// OpenBaoClusterClaimRestoreRequestStateSucceeded indicates the request completed successfully.
	OpenBaoClusterClaimRestoreRequestStateSucceeded OpenBaoClusterClaimRestoreRequestState = "Succeeded"
	// OpenBaoClusterClaimRestoreRequestStateBlocked indicates the request is outside the supported restore model.
	OpenBaoClusterClaimRestoreRequestStateBlocked OpenBaoClusterClaimRestoreRequestState = "Blocked"
	// OpenBaoClusterClaimRestoreRequestStateFailed indicates the request could not complete successfully.
	OpenBaoClusterClaimRestoreRequestStateFailed OpenBaoClusterClaimRestoreRequestState = "Failed"
)

// OpenBaoClusterClaimRestoreRequestSourceMode selects how a claim restore request resolves its snapshot source.
// +kubebuilder:validation:Enum=LatestSuccessful;BackupRequest
type OpenBaoClusterClaimRestoreRequestSourceMode string

const (
	// OpenBaoClusterClaimRestoreRequestSourceModeLatestSuccessful restores the latest successful backup recorded on the claim-managed local cluster.
	OpenBaoClusterClaimRestoreRequestSourceModeLatestSuccessful OpenBaoClusterClaimRestoreRequestSourceMode = "LatestSuccessful"
	// OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest restores the snapshot produced by a completed claim backup request.
	OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest OpenBaoClusterClaimRestoreRequestSourceMode = "BackupRequest"
)

// OpenBaoClusterClaimRestoreRequestSourceSpec selects the backup source for a claim restore request.
// +kubebuilder:validation:XValidation:rule="!has(self.backupRequestRef) || (has(self.mode) && self.mode == 'BackupRequest')",message="source.mode must be BackupRequest when source.backupRequestRef is set"
// +kubebuilder:validation:XValidation:rule="!has(self.mode) || self.mode != 'BackupRequest' || has(self.backupRequestRef)",message="source.backupRequestRef is required when source.mode is BackupRequest"
// +kubebuilder:validation:XValidation:rule="!has(self.mode) || self.mode != 'LatestSuccessful' || !has(self.backupRequestRef)",message="source.backupRequestRef must be omitted when source.mode is LatestSuccessful"
type OpenBaoClusterClaimRestoreRequestSourceSpec struct {
	// Mode selects how the restore source is resolved. Omitted mode defaults to LatestSuccessful.
	// +optional
	Mode OpenBaoClusterClaimRestoreRequestSourceMode `json:"mode,omitempty"`
	// BackupRequestRef identifies a completed OpenBaoClusterClaimBackupRequest for the same claim.
	// Required when mode is BackupRequest.
	// +optional
	BackupRequestRef *LocalReference `json:"backupRequestRef,omitempty"`
}

// OpenBaoClusterClaimRestoreRequestSpec defines the desired state of OpenBaoClusterClaimRestoreRequest.
type OpenBaoClusterClaimRestoreRequestSpec struct {
	// ClaimRef identifies the namespaced claim this request targets.
	ClaimRef LocalReference `json:"claimRef"`
	// Source selects the claim backup to restore. When omitted, the request restores the latest successful backup recorded on the claim-managed local cluster.
	// +optional
	Source *OpenBaoClusterClaimRestoreRequestSourceSpec `json:"source,omitempty"`
}

// OpenBaoClusterClaimRestoreRequestStatus defines the observed state of OpenBaoClusterClaimRestoreRequest.
type OpenBaoClusterClaimRestoreRequestStatus struct {
	// ObservedGeneration is the latest request generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// State is the current request workflow state.
	// +optional
	State OpenBaoClusterClaimRestoreRequestState `json:"state,omitempty"`
	// Reason explains the current workflow state.
	// +optional
	Reason string `json:"reason,omitempty"`
	// ClusterRef identifies the resolved local cluster targeted by this request.
	// +optional
	ClusterRef *NamespacedReference `json:"clusterRef,omitempty"`
	// RestoreRef identifies the underlying OpenBaoRestore execution object when one exists.
	// +optional
	RestoreRef *NamespacedReference `json:"restoreRef,omitempty"`
	// StartTime is when the restore attempt associated with this request started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// CompletionTime is when the request reached a terminal state.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// SnapshotKey identifies the snapshot object key restored by this request when available.
	// +optional
	SnapshotKey string `json:"snapshotKey,omitempty"`
	// Conditions represent the latest available observations of the request state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=obclaimrestore
// +kubebuilder:printcolumn:name="Claim",type="string",JSONPath=".spec.claimRef.name"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Snapshot",type="string",JSONPath=".status.snapshotKey",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoClusterClaimRestoreRequest is the immutable workflow request for one-shot same-cluster claim restores.
type OpenBaoClusterClaimRestoreRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoClusterClaimRestoreRequestSpec   `json:"spec"`
	Status OpenBaoClusterClaimRestoreRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoClusterClaimRestoreRequestList contains a list of OpenBaoClusterClaimRestoreRequest.
type OpenBaoClusterClaimRestoreRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoClusterClaimRestoreRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoClusterClaimRestoreRequest{}, &OpenBaoClusterClaimRestoreRequestList{})
}
