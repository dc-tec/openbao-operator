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

// OpenBaoClusterClaimUpgradeRequestState summarizes request progress.
// +kubebuilder:validation:Enum=Pending;RollingOut;Succeeded;Blocked;Failed
type OpenBaoClusterClaimUpgradeRequestState string

const (
	// OpenBaoClusterClaimUpgradeRequestStatePending indicates the request was admitted but not yet evaluated.
	OpenBaoClusterClaimUpgradeRequestStatePending OpenBaoClusterClaimUpgradeRequestState = "Pending"
	// OpenBaoClusterClaimUpgradeRequestStateRollingOut indicates the claim has entered rollout toward the classified target revision.
	OpenBaoClusterClaimUpgradeRequestStateRollingOut OpenBaoClusterClaimUpgradeRequestState = "RollingOut"
	// OpenBaoClusterClaimUpgradeRequestStateSucceeded indicates the request completed successfully.
	OpenBaoClusterClaimUpgradeRequestStateSucceeded OpenBaoClusterClaimUpgradeRequestState = "Succeeded"
	// OpenBaoClusterClaimUpgradeRequestStateBlocked indicates the requested change is outside the supported upgrade boundary.
	OpenBaoClusterClaimUpgradeRequestStateBlocked OpenBaoClusterClaimUpgradeRequestState = "Blocked"
	// OpenBaoClusterClaimUpgradeRequestStateFailed indicates the request could not be evaluated or executed successfully.
	OpenBaoClusterClaimUpgradeRequestStateFailed OpenBaoClusterClaimUpgradeRequestState = "Failed"
)

// OpenBaoClusterClaimUpgradeClassificationClass summarizes the evaluated compatibility class.
// +kubebuilder:validation:Enum=InPlace;Blocked
type OpenBaoClusterClaimUpgradeClassificationClass string

const (
	// OpenBaoClusterClaimUpgradeClassificationClassInPlace indicates the request can converge through the in-place upgrade path.
	OpenBaoClusterClaimUpgradeClassificationClassInPlace OpenBaoClusterClaimUpgradeClassificationClass = "InPlace"
	// OpenBaoClusterClaimUpgradeClassificationClassBlocked indicates the request is outside the supported upgrade model.
	OpenBaoClusterClaimUpgradeClassificationClassBlocked OpenBaoClusterClaimUpgradeClassificationClass = "Blocked"
)

// OpenBaoClusterClaimUpgradeRequestTargetSpec defines the requested upgrade target.
// +kubebuilder:validation:XValidation:rule="has(self.serviceOfferingRef) != has(self.serviceProfileRef)",message="exactly one of serviceOfferingRef or serviceProfileRef must be set"
type OpenBaoClusterClaimUpgradeRequestTargetSpec struct {
	// ServiceOfferingRef identifies the stable service-offering alias requested for upgrade.
	// +optional
	ServiceOfferingRef *LocalReference `json:"serviceOfferingRef,omitempty"`
	// ServiceProfileRef identifies the explicit immutable service-profile revision requested for upgrade.
	// +optional
	ServiceProfileRef *LocalReference `json:"serviceProfileRef,omitempty"`
}

// OpenBaoClusterClaimUpgradeRequestSpec defines the desired state of OpenBaoClusterClaimUpgradeRequest.
// +kubebuilder:validation:XValidation:rule="has(self.target.serviceOfferingRef) != has(self.target.serviceProfileRef)",message="exactly one of spec.target.serviceOfferingRef or spec.target.serviceProfileRef must be set"
type OpenBaoClusterClaimUpgradeRequestSpec struct {
	// ClaimRef identifies the namespaced claim this request targets.
	ClaimRef LocalReference `json:"claimRef"`
	// Target identifies the requested upgrade target.
	Target OpenBaoClusterClaimUpgradeRequestTargetSpec `json:"target"`
}

// OpenBaoClusterClaimUpgradeRequestRevisionStatus summarizes one current or target revision view.
type OpenBaoClusterClaimUpgradeRequestRevisionStatus struct {
	// ServiceOfferingRef identifies the stable offering alias involved in the request or applied state when relevant.
	// +optional
	ServiceOfferingRef *LocalReference `json:"serviceOfferingRef,omitempty"`
	// ServiceProfileRef identifies the resolved immutable service-profile revision.
	// +optional
	ServiceProfileRef *OpenBaoClusterClaimBoundRevisionReference `json:"serviceProfileRef,omitempty"`
	// ApprovedContract identifies the resolved approved service contract revision.
	// +optional
	ApprovedContract *OpenBaoClusterClaimContractIdentityStatus `json:"approvedContract,omitempty"`
	// RenderedContract identifies the resolved rendered execution contract revision when available.
	// +optional
	RenderedContract *OpenBaoClusterClaimContractIdentityStatus `json:"renderedContract,omitempty"`
}

// OpenBaoClusterClaimUpgradeRequestClassificationStatus summarizes compatibility evaluation.
type OpenBaoClusterClaimUpgradeRequestClassificationStatus struct {
	// Class is the evaluated compatibility class.
	// +optional
	Class OpenBaoClusterClaimUpgradeClassificationClass `json:"class,omitempty"`
	// Reason explains the evaluated compatibility class.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// OpenBaoClusterClaimUpgradeRequestStatus defines the observed state of OpenBaoClusterClaimUpgradeRequest.
type OpenBaoClusterClaimUpgradeRequestStatus struct {
	// ObservedGeneration is the latest request generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// State is the current request workflow state.
	// +optional
	State OpenBaoClusterClaimUpgradeRequestState `json:"state,omitempty"`
	// Reason explains the current workflow state.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Current summarizes the currently applied claim revision state when available.
	// +optional
	Current *OpenBaoClusterClaimUpgradeRequestRevisionStatus `json:"current,omitempty"`
	// Target summarizes the resolved target revision state when available.
	// +optional
	Target *OpenBaoClusterClaimUpgradeRequestRevisionStatus `json:"target,omitempty"`
	// Classification summarizes the evaluated compatibility class when available.
	// +optional
	Classification *OpenBaoClusterClaimUpgradeRequestClassificationStatus `json:"classification,omitempty"`
	// Conditions represent the latest available observations of the request state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=obclaimupgrade
// +kubebuilder:printcolumn:name="Claim",type="string",JSONPath=".spec.claimRef.name"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.reason"
// +kubebuilder:printcolumn:name="Class",type="string",JSONPath=".status.classification.class",priority=1
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".status.target.serviceProfileRef.name",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoClusterClaimUpgradeRequest is the immutable workflow request for post-materialization claim upgrades.
type OpenBaoClusterClaimUpgradeRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoClusterClaimUpgradeRequestSpec   `json:"spec"`
	Status OpenBaoClusterClaimUpgradeRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoClusterClaimUpgradeRequestList contains a list of OpenBaoClusterClaimUpgradeRequest.
type OpenBaoClusterClaimUpgradeRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoClusterClaimUpgradeRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoClusterClaimUpgradeRequest{}, &OpenBaoClusterClaimUpgradeRequestList{})
}
