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

// OpenBaoServiceOfferingLifecycleState summarizes the lifecycle posture of a stable service-offering alias.
// +kubebuilder:validation:Enum=Active;Deprecated;Replaced
type OpenBaoServiceOfferingLifecycleState string

const (
	// OpenBaoServiceOfferingLifecycleStateActive indicates the offering remains recommended for new bindings.
	OpenBaoServiceOfferingLifecycleStateActive OpenBaoServiceOfferingLifecycleState = "Active"
	// OpenBaoServiceOfferingLifecycleStateDeprecated indicates the offering remains usable but is no longer preferred.
	OpenBaoServiceOfferingLifecycleStateDeprecated OpenBaoServiceOfferingLifecycleState = "Deprecated"
	// OpenBaoServiceOfferingLifecycleStateReplaced indicates the offering has a preferred replacement for new bindings.
	OpenBaoServiceOfferingLifecycleStateReplaced OpenBaoServiceOfferingLifecycleState = "Replaced"
)

// OpenBaoServiceOfferingLifecycleSpec captures optional lifecycle metadata for a stable service-offering alias.
type OpenBaoServiceOfferingLifecycleSpec struct {
	// State summarizes whether the offering remains active for new bindings.
	// +kubebuilder:default=Active
	// +optional
	State OpenBaoServiceOfferingLifecycleState `json:"state,omitempty"`
	// ReplacementRef identifies the preferred replacement offering when this one is deprecated or replaced.
	// +optional
	ReplacementRef *LocalReference `json:"replacementRef,omitempty"`
}

// OpenBaoServiceOfferingSpec defines the desired state of OpenBaoServiceOffering.
type OpenBaoServiceOfferingSpec struct {
	// CurrentRevisionRef identifies the current immutable service-profile revision for new claim bindings.
	CurrentRevisionRef LocalReference `json:"currentRevisionRef"`
	// Lifecycle captures optional lifecycle metadata for this stable offering alias.
	// +optional
	Lifecycle *OpenBaoServiceOfferingLifecycleSpec `json:"lifecycle,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Revision",type="string",JSONPath=".spec.currentRevisionRef.name"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".spec.lifecycle.state"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoServiceOffering is the mutable stable alias that points claim users at a current immutable service-profile revision.
type OpenBaoServiceOffering struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoServiceOfferingSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoServiceOfferingList contains a list of OpenBaoServiceOffering.
type OpenBaoServiceOfferingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoServiceOffering `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoServiceOffering{}, &OpenBaoServiceOfferingList{})
}
