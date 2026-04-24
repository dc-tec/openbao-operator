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

// OpenBaoObservabilityProfileSpec defines the desired state of OpenBaoObservabilityProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoObservabilityProfileSpec struct {
	// Observability configures metrics and ServiceMonitor integration.
	// +optional
	Observability *ObservabilityConfig `json:"observability,omitempty"`
	// Telemetry configures OpenBao telemetry reporting.
	// +optional
	Telemetry *TelemetryConfig `json:"telemetry,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Metrics",type="boolean",JSONPath=".spec.observability.metrics.enabled"
// +kubebuilder:printcolumn:name="ServiceMonitor",type="boolean",JSONPath=".spec.observability.metrics.serviceMonitor.enabled"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoObservabilityProfile is the immutable platform-owned observability catalog object.
type OpenBaoObservabilityProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoObservabilityProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoObservabilityProfileList contains a list of OpenBaoObservabilityProfile.
type OpenBaoObservabilityProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoObservabilityProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoObservabilityProfile{}, &OpenBaoObservabilityProfileList{})
}
