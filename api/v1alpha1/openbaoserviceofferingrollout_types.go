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

// OpenBaoServiceOfferingRolloutState summarizes rollout progress.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Blocked;Failed
type OpenBaoServiceOfferingRolloutState string

const (
	// OpenBaoServiceOfferingRolloutStatePending indicates the rollout has not yet started.
	OpenBaoServiceOfferingRolloutStatePending OpenBaoServiceOfferingRolloutState = "Pending"
	// OpenBaoServiceOfferingRolloutStateRunning indicates the rollout is creating or waiting for claim upgrade requests.
	OpenBaoServiceOfferingRolloutStateRunning OpenBaoServiceOfferingRolloutState = "Running"
	// OpenBaoServiceOfferingRolloutStateSucceeded indicates all selected claims are on the target revision.
	OpenBaoServiceOfferingRolloutStateSucceeded OpenBaoServiceOfferingRolloutState = "Succeeded"
	// OpenBaoServiceOfferingRolloutStateBlocked indicates rollout intent is valid but cannot proceed without operator action.
	OpenBaoServiceOfferingRolloutStateBlocked OpenBaoServiceOfferingRolloutState = "Blocked"
	// OpenBaoServiceOfferingRolloutStateFailed indicates the rollout controller could not evaluate or create required requests.
	OpenBaoServiceOfferingRolloutStateFailed OpenBaoServiceOfferingRolloutState = "Failed"
)

// OpenBaoServiceOfferingRolloutMode controls which upgrade classes the rollout may drive.
// +kubebuilder:validation:Enum=InPlaceOnly
type OpenBaoServiceOfferingRolloutMode string

const (
	// OpenBaoServiceOfferingRolloutModeInPlaceOnly limits rollouts to the existing in-place claim upgrade workflow.
	OpenBaoServiceOfferingRolloutModeInPlaceOnly OpenBaoServiceOfferingRolloutMode = "InPlaceOnly"
)

// OpenBaoServiceOfferingRolloutSelectorSpec selects claims bound to the offering.
type OpenBaoServiceOfferingRolloutSelectorSpec struct {
	// Namespaces restricts the rollout to explicitly named claim namespaces. Empty selects all namespaces.
	// +listType=set
	// +optional
	Namespaces []string `json:"namespaces,omitempty"`
	// ClaimSelector restricts the rollout to claims with matching labels.
	// +optional
	ClaimSelector *metav1.LabelSelector `json:"claimSelector,omitempty"`
}

// OpenBaoServiceOfferingRolloutStrategySpec controls rollout orchestration.
type OpenBaoServiceOfferingRolloutStrategySpec struct {
	// MaxConcurrent limits the number of active claim upgrade requests created by this rollout.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxConcurrent *int32 `json:"maxConcurrent,omitempty"`
	// Mode controls which upgrade classes the rollout may drive.
	// +kubebuilder:default=InPlaceOnly
	// +optional
	Mode OpenBaoServiceOfferingRolloutMode `json:"mode,omitempty"`
}

// OpenBaoServiceOfferingRolloutSpec defines the desired state of OpenBaoServiceOfferingRollout.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoServiceOfferingRolloutSpec struct {
	// OfferingRef identifies the stable service-offering alias to roll out.
	OfferingRef LocalReference `json:"offeringRef"`
	// TargetRevisionRef identifies the immutable service-profile revision that the offering must currently point at.
	TargetRevisionRef LocalReference `json:"targetRevisionRef"`
	// Selector restricts which claims bound to the offering are included.
	// +optional
	Selector *OpenBaoServiceOfferingRolloutSelectorSpec `json:"selector,omitempty"`
	// Strategy controls rollout orchestration.
	// +optional
	Strategy *OpenBaoServiceOfferingRolloutStrategySpec `json:"strategy,omitempty"`
}

// OpenBaoServiceOfferingRolloutClaimStatus summarizes rollout progress for one selected claim.
type OpenBaoServiceOfferingRolloutClaimStatus struct {
	// Namespace is the claim namespace.
	Namespace string `json:"namespace"`
	// Name is the claim name.
	Name string `json:"name"`
	// RequestRef identifies the claim upgrade request created for this claim when one exists.
	// +optional
	RequestRef *NamespacedReference `json:"requestRef,omitempty"`
	// State is the rollout state observed for this claim.
	// +optional
	State OpenBaoClusterClaimUpgradeRequestState `json:"state,omitempty"`
	// Reason explains the per-claim rollout state.
	// +optional
	Reason string `json:"reason,omitempty"`
}

// OpenBaoServiceOfferingRolloutStatus defines the observed state of OpenBaoServiceOfferingRollout.
type OpenBaoServiceOfferingRolloutStatus struct {
	// ObservedGeneration is the latest rollout generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// State is the current rollout workflow state.
	// +optional
	State OpenBaoServiceOfferingRolloutState `json:"state,omitempty"`
	// Reason explains the current rollout workflow state.
	// +optional
	Reason string `json:"reason,omitempty"`
	// TargetRevisionRef identifies the resolved immutable target revision.
	// +optional
	TargetRevisionRef *OpenBaoClusterClaimBoundRevisionReference `json:"targetRevisionRef,omitempty"`
	// Total is the number of claims selected by this rollout.
	// +optional
	Total int32 `json:"total,omitempty"`
	// Pending is the number of selected claims waiting for a rollout slot or request evaluation.
	// +optional
	Pending int32 `json:"pending,omitempty"`
	// Running is the number of selected claims with an active rollout request.
	// +optional
	Running int32 `json:"running,omitempty"`
	// Succeeded is the number of selected claims that reached the target revision.
	// +optional
	Succeeded int32 `json:"succeeded,omitempty"`
	// Blocked is the number of selected claims whose upgrade request is blocked.
	// +optional
	Blocked int32 `json:"blocked,omitempty"`
	// Failed is the number of selected claims whose upgrade request failed.
	// +optional
	Failed int32 `json:"failed,omitempty"`
	// Claims summarizes selected claim rollout progress.
	// +listType=map
	// +listMapKey=namespace
	// +listMapKey=name
	// +optional
	Claims []OpenBaoServiceOfferingRolloutClaimStatus `json:"claims,omitempty"`
	// Conditions represent the latest available observations of the rollout state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=obofferingrollout
// +kubebuilder:printcolumn:name="Offering",type="string",JSONPath=".spec.offeringRef.name"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetRevisionRef.name"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.total",priority=1
// +kubebuilder:printcolumn:name="Succeeded",type="integer",JSONPath=".status.succeeded",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoServiceOfferingRollout is an admin-owned rollout intent for promoting claims bound to a service offering.
type OpenBaoServiceOfferingRollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenBaoServiceOfferingRolloutSpec   `json:"spec"`
	Status OpenBaoServiceOfferingRolloutStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenBaoServiceOfferingRolloutList contains a list of OpenBaoServiceOfferingRollout.
type OpenBaoServiceOfferingRolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoServiceOfferingRollout `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoServiceOfferingRollout{}, &OpenBaoServiceOfferingRolloutList{})
}
