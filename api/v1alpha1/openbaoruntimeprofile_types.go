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

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenBaoRuntimeProfileSpec defines the desired state of OpenBaoRuntimeProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoRuntimeProfileSpec struct {
	// ServiceAccount configures the Kubernetes ServiceAccount used by OpenBao Pods.
	// +optional
	ServiceAccount *ServiceAccountConfig `json:"serviceAccount,omitempty"`
	// PodMetadata configures additional labels and annotations for OpenBao Pods.
	// +optional
	PodMetadata *PodMetadataConfig `json:"podMetadata,omitempty"`
	// ImagePullSecrets lists same-namespace pull secrets for operator-managed images.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// ImageVerification configures supply chain checks for the OpenBao workload image.
	// +optional
	ImageVerification *ImageVerificationConfig `json:"imageVerification,omitempty"`
	// OperatorImageVerification configures supply chain checks for operator-managed helper images.
	// +optional
	OperatorImageVerification *ImageVerificationConfig `json:"operatorImageVerification,omitempty"`
	// WorkloadHardening configures opt-in workload hardening features.
	// +optional
	WorkloadHardening *WorkloadHardeningConfig `json:"workloadHardening,omitempty"`
	// SecurityContext configures the PodSecurityContext for OpenBao Pods.
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// HelperImages configures platform-approved helper images used by lifecycle executors.
	// +optional
	HelperImages *OpenBaoRuntimeProfileHelperImagesSpec `json:"helperImages,omitempty"`
	// ReadReplica configures runtime settings for read replicas.
	// +optional
	ReadReplica *OpenBaoRuntimeProfileReadReplicaSpec `json:"readReplica,omitempty"`
}

// OpenBaoRuntimeProfileHelperImagesSpec defines catalog-approved helper images.
type OpenBaoRuntimeProfileHelperImagesSpec struct {
	// Init overrides the init helper image.
	// +optional
	Init string `json:"init,omitempty"`
	// Backup overrides the backup helper image.
	// +optional
	Backup string `json:"backup,omitempty"`
	// Restore overrides the restore helper image.
	// +optional
	Restore string `json:"restore,omitempty"`
	// Upgrade overrides the upgrade helper image.
	// +optional
	Upgrade string `json:"upgrade,omitempty"`
}

// OpenBaoRuntimeProfileReadReplicaSpec defines read-replica runtime settings.
type OpenBaoRuntimeProfileReadReplicaSpec struct {
	// Template applies read-replica pod metadata, resources, and scheduling.
	// +optional
	Template *ReadReplicaTemplateConfig `json:"template,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="ServiceAccount",type="string",JSONPath=".spec.serviceAccount.name"
// +kubebuilder:printcolumn:name="AppArmor",type="boolean",JSONPath=".spec.workloadHardening.appArmorEnabled"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoRuntimeProfile is the immutable platform-owned runtime integration catalog object.
type OpenBaoRuntimeProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoRuntimeProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoRuntimeProfileList contains a list of OpenBaoRuntimeProfile.
type OpenBaoRuntimeProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoRuntimeProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoRuntimeProfile{}, &OpenBaoRuntimeProfileList{})
}
