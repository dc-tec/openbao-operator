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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OpenBaoBackupBackendDriver identifies the concrete backup backend family.
// +kubebuilder:validation:Enum=ObjectStorage
type OpenBaoBackupBackendDriver string

const (
	// OpenBaoBackupBackendDriverObjectStorage identifies an object-storage-compatible backup backend.
	OpenBaoBackupBackendDriverObjectStorage OpenBaoBackupBackendDriver = "ObjectStorage"
)

// OpenBaoObjectStorageProvider identifies the concrete object-storage provider family.
// +kubebuilder:validation:Enum=s3;gcs;azure
type OpenBaoObjectStorageProvider string

const (
	// OpenBaoObjectStorageProviderS3 identifies an S3-compatible object store.
	OpenBaoObjectStorageProviderS3 OpenBaoObjectStorageProvider = "s3"
	// OpenBaoObjectStorageProviderGCS identifies Google Cloud Storage.
	OpenBaoObjectStorageProviderGCS OpenBaoObjectStorageProvider = "gcs"
	// OpenBaoObjectStorageProviderAzure identifies Azure Blob Storage.
	OpenBaoObjectStorageProviderAzure OpenBaoObjectStorageProvider = "azure"
)

// OpenBaoBackupBackendObjectStorageSpec defines concrete object-storage connectivity and protocol posture.
type OpenBaoBackupBackendObjectStorageSpec struct {
	// Provider identifies the concrete object-storage provider family.
	Provider OpenBaoObjectStorageProvider `json:"provider"`
	// Endpoint is the HTTP(S) endpoint for the object storage service when required.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Region is the provider region when required.
	// +optional
	Region string `json:"region,omitempty"`
	// UsePathStyle identifies whether S3-compatible path-style addressing is required.
	// +optional
	UsePathStyle bool `json:"usePathStyle,omitempty"`
	// GCSProject identifies the GCP project when Provider is gcs and the platform wants to pin it explicitly.
	// +optional
	GCSProject string `json:"gcsProject,omitempty"`
	// AzureStorageAccount identifies the Azure storage account when Provider is azure.
	// +optional
	AzureStorageAccount string `json:"azureStorageAccount,omitempty"`
	// AzureContainer identifies the Azure Blob container override when it should not follow the rendered location.
	// +optional
	AzureContainer string `json:"azureContainer,omitempty"`
	// InsecureSkipVerify identifies whether TLS verification should be skipped for this backend.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// RequiredEgressRules carries the concrete additional egress rules claim-managed
	// workloads need so backup jobs can reach this backend when explicit
	// allow-listing is required.
	//
	// These rules are execution-facing runtime inputs owned by the immutable backup
	// backend object. They are not claim-authored network policy.
	// +optional
	RequiredEgressRules []networkingv1.NetworkPolicyEgressRule `json:"requiredEgressRules,omitempty"`
}

// OpenBaoBackupBackendSpec defines the desired state of OpenBaoBackupBackend.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
// +kubebuilder:validation:XValidation:rule="self.driver != 'ObjectStorage' || has(self.objectStorage)",message="spec.objectStorage is required when driver is ObjectStorage"
type OpenBaoBackupBackendSpec struct {
	// Driver identifies the concrete backup backend family.
	Driver OpenBaoBackupBackendDriver `json:"driver"`
	// ObjectStorage carries concrete object-storage connectivity and protocol posture.
	// +optional
	ObjectStorage *OpenBaoBackupBackendObjectStorageSpec `json:"objectStorage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Driver",type="string",JSONPath=".spec.driver"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.objectStorage.provider"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoBackupBackend is the immutable platform-owned concrete backup backend object.
type OpenBaoBackupBackend struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoBackupBackendSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoBackupBackendList contains a list of OpenBaoBackupBackend.
type OpenBaoBackupBackendList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoBackupBackend `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoBackupBackend{}, &OpenBaoBackupBackendList{})
}
