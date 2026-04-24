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

// OpenBaoEntrypointMode identifies the reusable network anchor type.
// +kubebuilder:validation:Enum=Gateway;Ingress;Service
type OpenBaoEntrypointMode string

const (
	// OpenBaoEntrypointModeGateway identifies a Gateway API entrypoint.
	OpenBaoEntrypointModeGateway OpenBaoEntrypointMode = "Gateway"
	// OpenBaoEntrypointModeIngress identifies an ingress-backed entrypoint.
	OpenBaoEntrypointModeIngress OpenBaoEntrypointMode = "Ingress"
	// OpenBaoEntrypointModeService identifies a direct Service-backed entrypoint.
	OpenBaoEntrypointModeService OpenBaoEntrypointMode = "Service"
)

// OpenBaoEntrypointObjectReference identifies the concrete Kubernetes object that anchors the entrypoint.
type OpenBaoEntrypointObjectReference struct {
	// APIGroup is the API group of the referenced object.
	// +kubebuilder:validation:MinLength=1
	APIGroup string `json:"apiGroup"`
	// Kind is the referenced object kind.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`
	// Name is the referenced object name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace is the referenced object namespace when applicable.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// OpenBaoEntrypointListenerPolicySpec defines listener or section binding posture.
type OpenBaoEntrypointListenerPolicySpec struct {
	// SectionName identifies the listener or route section to bind when required.
	// +optional
	SectionName string `json:"sectionName,omitempty"`
}

// OpenBaoEntrypointSpec defines the desired state of OpenBaoEntrypoint.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoEntrypointSpec struct {
	// Mode identifies the reusable network anchor type.
	Mode OpenBaoEntrypointMode `json:"mode"`
	// ObjectRef identifies the concrete Kubernetes object that anchors this entrypoint.
	ObjectRef OpenBaoEntrypointObjectReference `json:"objectRef"`
	// ListenerPolicy defines listener or section binding posture.
	// +optional
	ListenerPolicy *OpenBaoEntrypointListenerPolicySpec `json:"listenerPolicy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode"
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.objectRef.kind"
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.objectRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoEntrypoint is the immutable platform-owned network entrypoint catalog object.
type OpenBaoEntrypoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoEntrypointSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoEntrypointList contains a list of OpenBaoEntrypoint.
type OpenBaoEntrypointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoEntrypoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoEntrypoint{}, &OpenBaoEntrypointList{})
}
