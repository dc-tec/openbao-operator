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

// OpenBaoIngressBackendTLSPublicationMode identifies how a concrete ingress
// controller expresses backend TLS behavior.
// +kubebuilder:validation:Enum=None;Annotation
type OpenBaoIngressBackendTLSPublicationMode string

const (
	// OpenBaoIngressBackendTLSPublicationModeNone indicates the selected ingress
	// controller does not require a separate backend-TLS publication mechanism.
	OpenBaoIngressBackendTLSPublicationModeNone OpenBaoIngressBackendTLSPublicationMode = "None"
	// OpenBaoIngressBackendTLSPublicationModeAnnotation indicates the selected ingress
	// controller requires annotations on the managed Ingress resource.
	OpenBaoIngressBackendTLSPublicationModeAnnotation OpenBaoIngressBackendTLSPublicationMode = "Annotation"
)

// OpenBaoIngressPolicyBackendTLSSpec defines how a concrete ingress controller
// expects backend TLS behavior to be published.
type OpenBaoIngressPolicyBackendTLSSpec struct {
	// PublicationMode identifies how backend TLS posture is expressed for this
	// ingress controller.
	// +kubebuilder:default=None
	// +optional
	PublicationMode OpenBaoIngressBackendTLSPublicationMode `json:"publicationMode,omitempty"`
}

// OpenBaoIngressPolicySpec defines the desired state of OpenBaoIngressPolicy.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoIngressPolicySpec struct {
	// PathType identifies how the ingress controller should interpret the route path.
	// +kubebuilder:default=Prefix
	// +optional
	PathType IngressPathType `json:"pathType,omitempty"`
	// Annotations are the concrete controller-specific annotations applied to the
	// managed Ingress resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// BackendTLS defines how backend TLS posture is published for the selected
	// ingress controller.
	// +optional
	BackendTLS *OpenBaoIngressPolicyBackendTLSSpec `json:"backendTLS,omitempty"`
	// ReadinessMode identifies how the operator should decide whether ingress
	// integration is ready for claim-facing endpoint publication.
	// +kubebuilder:default=LoadBalancerPublished
	// +optional
	ReadinessMode IngressReadinessMode `json:"readinessMode,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="PathType",type="string",JSONPath=".spec.pathType"
// +kubebuilder:printcolumn:name="Readiness",type="string",JSONPath=".spec.readinessMode"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoIngressPolicy is the immutable platform-owned ingress publication policy object.
type OpenBaoIngressPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoIngressPolicySpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoIngressPolicyList contains a list of OpenBaoIngressPolicy.
type OpenBaoIngressPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoIngressPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoIngressPolicy{}, &OpenBaoIngressPolicyList{})
}
