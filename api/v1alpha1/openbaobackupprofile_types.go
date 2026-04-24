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

// OpenBaoBackupProfileSpec defines the desired state of OpenBaoBackupProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoBackupProfileSpec struct {
	// Schedule is the recurring backup schedule when backups are enabled.
	// +optional
	Schedule string `json:"schedule,omitempty"`
	// Retention defines backup retention posture.
	// +optional
	Retention *BackupRetention `json:"retention,omitempty"`
	// TargetRef identifies the immutable backup target policy revision when backups are enabled.
	// +optional
	TargetRef *LocalReference `json:"targetRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule"
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".spec.targetRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoBackupProfile is the immutable platform-owned backup catalog object.
type OpenBaoBackupProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoBackupProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoBackupProfileList contains a list of OpenBaoBackupProfile.
type OpenBaoBackupProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoBackupProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoBackupProfile{}, &OpenBaoBackupProfileList{})
}
