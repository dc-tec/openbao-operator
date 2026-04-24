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

// OpenBaoStorageProfileVolumeSpec defines a platform-owned storage class posture.
type OpenBaoStorageProfileVolumeSpec struct {
	// StorageClassName is an optional StorageClass for the PVCs.
	// When omitted, the cluster default StorageClass is used.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// OpenBaoStorageProfileReadReplicaSpec defines storage-class posture for read replicas.
type OpenBaoStorageProfileReadReplicaSpec struct {
	// StorageClassName is an optional StorageClass for read-replica PVCs.
	// When omitted and UsePrimaryStorageClass is true or omitted, read replicas
	// inherit the primary storage class.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
	// UsePrimaryStorageClass controls whether read replicas inherit the primary
	// storage class when StorageClassName is omitted.
	// +kubebuilder:default=true
	// +optional
	UsePrimaryStorageClass *bool `json:"usePrimaryStorageClass,omitempty"`
}

// OpenBaoStorageProfileACMECacheSpec defines storage posture for OpenBao native
// ACME account and certificate cache data.
// +kubebuilder:validation:XValidation:rule="self.mode != 'ManagedPVC' || !has(self.existingClaimName) || size(self.existingClaimName) == 0",message="spec.acmeCache.existingClaimName is only supported when mode is ExistingPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || size(self.existingClaimName) > 0",message="spec.acmeCache.existingClaimName is required when mode is ExistingPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || !has(self.size) || size(self.size) == 0",message="spec.acmeCache.size is only supported when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ExistingPVC' || !has(self.storageClassName) || size(self.storageClassName) == 0",message="spec.acmeCache.storageClassName is only supported when mode is ManagedPVC"
// +kubebuilder:validation:XValidation:rule="self.mode != 'ManagedPVC' || size(self.size) > 0",message="spec.acmeCache.size is required when mode is ManagedPVC"
type OpenBaoStorageProfileACMECacheSpec struct {
	// Mode selects whether the operator creates a dedicated RWX PVC or mounts an existing one.
	Mode ACMESharedCacheMode `json:"mode"`
	// ExistingClaimName is the name of a pre-created RWX PVC in the same namespace.
	// Required when Mode is ExistingPVC.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ExistingClaimName string `json:"existingClaimName,omitempty"`
	// Size is the requested capacity for the managed ACME cache PVC.
	// Required when Mode is ManagedPVC.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Size string `json:"size,omitempty"`
	// StorageClassName is an optional StorageClass for the managed ACME cache PVC.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// OpenBaoStorageProfileSpec defines the desired state of OpenBaoStorageProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoStorageProfileSpec struct {
	// Primary defines primary Raft PVC storage-class posture.
	// +optional
	Primary *OpenBaoStorageProfileVolumeSpec `json:"primary,omitempty"`
	// ReadReplica defines read-replica PVC storage-class posture.
	// +optional
	ReadReplica *OpenBaoStorageProfileReadReplicaSpec `json:"readReplica,omitempty"`
	// ACMECache defines the shared RWX filesystem cache used by OpenBao native
	// ACME listener TLS when selected by the exposure policy.
	// +optional
	ACMECache *OpenBaoStorageProfileACMECacheSpec `json:"acmeCache,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="PrimaryClass",type="string",JSONPath=".spec.primary.storageClassName"
// +kubebuilder:printcolumn:name="ReadClass",type="string",JSONPath=".spec.readReplica.storageClassName"
// +kubebuilder:printcolumn:name="ACMECache",type="string",JSONPath=".spec.acmeCache.mode"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoStorageProfile is the immutable platform-owned storage implementation catalog object.
type OpenBaoStorageProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoStorageProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoStorageProfileList contains a list of OpenBaoStorageProfile.
type OpenBaoStorageProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoStorageProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoStorageProfile{}, &OpenBaoStorageProfileList{})
}
