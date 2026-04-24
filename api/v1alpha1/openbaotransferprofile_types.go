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

// OpenBaoTransferProfileSpec defines the desired state of OpenBaoTransferProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoTransferProfileSpec struct {
	// PartSize is the multipart upload part size in bytes.
	// +kubebuilder:default=10485760
	// +kubebuilder:validation:Minimum=5242880
	// +optional
	PartSize int64 `json:"partSize,omitempty"`
	// Concurrency is the number of concurrent multipart uploads.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +optional
	Concurrency int32 `json:"concurrency,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Part Size",type="integer",JSONPath=".spec.partSize"
// +kubebuilder:printcolumn:name="Concurrency",type="integer",JSONPath=".spec.concurrency"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoTransferProfile is the immutable platform-owned transfer tuning object.
type OpenBaoTransferProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoTransferProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoTransferProfileList contains a list of OpenBaoTransferProfile.
type OpenBaoTransferProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoTransferProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoTransferProfile{}, &OpenBaoTransferProfileList{})
}
