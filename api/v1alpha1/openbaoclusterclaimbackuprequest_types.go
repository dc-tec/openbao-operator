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

// OpenBaoClusterClaimBackupRequestState summarizes request progress.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Blocked;Failed
type OpenBaoClusterClaimBackupRequestState string

const (
	// OpenBaoClusterClaimBackupRequestStatePending indicates the request was admitted but has not completed yet.
	OpenBaoClusterClaimBackupRequestStatePending OpenBaoClusterClaimBackupRequestState = "Pending"
	// OpenBaoClusterClaimBackupRequestStateRunning indicates the backup workflow is actively converging.
	OpenBaoClusterClaimBackupRequestStateRunning OpenBaoClusterClaimBackupRequestState = "Running"
	// OpenBaoClusterClaimBackupRequestStateSucceeded indicates the request completed successfully.
	OpenBaoClusterClaimBackupRequestStateSucceeded OpenBaoClusterClaimBackupRequestState = "Succeeded"
	// OpenBaoClusterClaimBackupRequestStateBlocked indicates the request is outside the supported backup model.
	OpenBaoClusterClaimBackupRequestStateBlocked OpenBaoClusterClaimBackupRequestState = "Blocked"
	// OpenBaoClusterClaimBackupRequestStateFailed indicates the request could not complete successfully.
	OpenBaoClusterClaimBackupRequestStateFailed OpenBaoClusterClaimBackupRequestState = "Failed"
)

// OpenBaoClusterClaimBackupRequestSpec defines the desired state of OpenBaoClusterClaimBackupRequest.
type OpenBaoClusterClaimBackupRequestSpec struct {
	// ClaimRef identifies the namespaced claim this request targets.
	ClaimRef LocalReference `json:"claimRef"`
}

// OpenBaoClusterClaimBackupRequestStatus defines the observed state of OpenBaoClusterClaimBackupRequest.
type OpenBaoClusterClaimBackupRequestStatus struct {
	// ObservedGeneration is the latest request generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// State is the current request workflow state.
	// +optional
	State OpenBaoClusterClaimBackupRequestState `json:"state,omitempty"`
	// Reason explains the current workflow state.
	// +optional
	Reason string `json:"reason,omitempty"`
	// ClusterRef identifies the resolved local cluster targeted by this request.
	// +optional
	ClusterRef *NamespacedReference `json:"clusterRef,omitempty"`
	// StartTime is when the backup attempt associated with this request started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// CompletionTime is when the request reached a terminal state.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// SnapshotKey identifies the successful snapshot object key when available.
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
// +kubebuilder:resource:shortName=obclaimbackup
// +kubebuilder:printcolumn:name="Claim",type="string",JSONPath=".spec.claimRef.name"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Snapshot",type="string",JSONPath=".status.snapshotKey",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoClusterClaimBackupRequest is the immutable workflow request for one-shot same-cluster claim backups.
type OpenBaoClusterClaimBackupRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoClusterClaimBackupRequestSpec   `json:"spec"`
	Status OpenBaoClusterClaimBackupRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoClusterClaimBackupRequestList contains a list of OpenBaoClusterClaimBackupRequest.
type OpenBaoClusterClaimBackupRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoClusterClaimBackupRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoClusterClaimBackupRequest{}, &OpenBaoClusterClaimBackupRequestList{})
}
