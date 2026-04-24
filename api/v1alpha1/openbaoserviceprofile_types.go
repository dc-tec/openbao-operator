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

// OpenBaoBootstrapMode identifies the approved bootstrap posture for a service profile.
// +kubebuilder:validation:Enum=ManagedInit;SelfInit
type OpenBaoBootstrapMode string

const (
	// OpenBaoBootstrapModeManagedInit uses operator-managed initialization flows.
	OpenBaoBootstrapModeManagedInit OpenBaoBootstrapMode = "ManagedInit"
	// OpenBaoBootstrapModeSelfInit uses OpenBao self-initialization.
	OpenBaoBootstrapModeSelfInit OpenBaoBootstrapMode = "SelfInit"
)

// OpenBaoServiceProfileClusterSpec defines the approved cluster service shape.
type OpenBaoServiceProfileClusterSpec struct {
	// Version is the approved OpenBao version.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`
	// Voters is the approved voter replica count.
	// +kubebuilder:validation:Minimum=1
	Voters int32 `json:"voters"`
	// ReadReplicas is the approved steady-state read-replica count.
	// +kubebuilder:validation:Minimum=0
	// +optional
	ReadReplicas *int32 `json:"readReplicas,omitempty"`
	// SecurityProfile is the approved cluster security posture.
	SecurityProfile Profile `json:"securityProfile"`
}

// OpenBaoServiceProfileStorageSpec defines approved storage capacities for the service.
type OpenBaoServiceProfileStorageSpec struct {
	// PrimarySize is the approved primary storage size.
	// +kubebuilder:validation:MinLength=1
	PrimarySize string `json:"primarySize"`
	// ReadReplicaSize is the approved read-replica storage size.
	// +kubebuilder:validation:MinLength=1
	// +optional
	ReadReplicaSize string `json:"readReplicaSize,omitempty"`
	// ProfileRef identifies the selected immutable storage implementation profile revision.
	// Capacity remains part of the service profile; storage classes and auxiliary storage
	// implementation details live in the referenced profile.
	// +optional
	ProfileRef *LocalReference `json:"profileRef,omitempty"`
}

// OpenBaoServiceProfileBootstrapSpec defines the approved bootstrap posture.
type OpenBaoServiceProfileBootstrapSpec struct {
	// Mode identifies the approved bootstrap mode.
	Mode OpenBaoBootstrapMode `json:"mode"`
	// ProfileRef identifies the selected immutable bootstrap profile revision when needed.
	// +optional
	ProfileRef *LocalReference `json:"profileRef,omitempty"`
}

// OpenBaoServiceProfileExposureSpec defines the approved exposure posture.
type OpenBaoServiceProfileExposureSpec struct {
	// ClassRef identifies the selected immutable exposure class revision.
	ClassRef LocalReference `json:"classRef"`
}

// OpenBaoServiceProfileUnsealSpec defines the approved unseal posture.
type OpenBaoServiceProfileUnsealSpec struct {
	// ProfileRef identifies the selected immutable unseal implementation profile revision.
	// When omitted, Development profiles use operator-managed static unseal and Hardened
	// profiles use the operator's same-cluster transit defaults.
	// +optional
	ProfileRef *LocalReference `json:"profileRef,omitempty"`
}

// OpenBaoServiceProfileRuntimeSpec defines the approved runtime integration posture.
type OpenBaoServiceProfileRuntimeSpec struct {
	// ProfileRef identifies the selected immutable runtime implementation profile revision.
	// +optional
	ProfileRef *LocalReference `json:"profileRef,omitempty"`
}

// OpenBaoServiceProfileObservabilitySpec defines the approved observability posture.
type OpenBaoServiceProfileObservabilitySpec struct {
	// ProfileRef identifies the selected immutable observability implementation profile revision.
	// +optional
	ProfileRef *LocalReference `json:"profileRef,omitempty"`
}

// OpenBaoServiceProfileNetworkSpec defines the approved network dependency posture.
type OpenBaoServiceProfileNetworkSpec struct {
	// ProfileRef identifies the selected immutable network implementation profile revision.
	// +optional
	ProfileRef *LocalReference `json:"profileRef,omitempty"`
}

// OpenBaoServiceProfileBackupSpec defines the approved backup posture.
type OpenBaoServiceProfileBackupSpec struct {
	// ProfileRef identifies the selected immutable backup profile revision.
	ProfileRef LocalReference `json:"profileRef"`
}

// OpenBaoServiceProfileLifecycleSpec defines approved steady-state lifecycle policy.
type OpenBaoServiceProfileLifecycleSpec struct {
	// PolicyRef identifies the selected immutable upgrade-policy revision.
	// +optional
	PolicyRef *LocalReference `json:"policyRef,omitempty"`
	// UpgradeStrategy identifies the approved upgrade strategy.
	// +kubebuilder:default=RollingUpdate
	// +optional
	UpgradeStrategy UpdateStrategyType `json:"upgradeStrategy,omitempty"`
	// PreUpgradeSnapshot identifies whether upgrades should take a safety snapshot.
	// +optional
	PreUpgradeSnapshot *bool `json:"preUpgradeSnapshot,omitempty"`
}

// OpenBaoServiceProfileSpec defines the desired state of OpenBaoServiceProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoServiceProfileSpec struct {
	// Cluster defines the approved cluster service shape.
	Cluster OpenBaoServiceProfileClusterSpec `json:"cluster"`
	// Storage defines the approved service storage capacities.
	Storage OpenBaoServiceProfileStorageSpec `json:"storage"`
	// Bootstrap defines the approved bootstrap posture.
	Bootstrap OpenBaoServiceProfileBootstrapSpec `json:"bootstrap"`
	// Exposure defines the approved exposure posture.
	Exposure OpenBaoServiceProfileExposureSpec `json:"exposure"`
	// Unseal defines the approved unseal posture.
	// +optional
	Unseal *OpenBaoServiceProfileUnsealSpec `json:"unseal,omitempty"`
	// Runtime defines the approved workload runtime integration posture.
	// +optional
	Runtime *OpenBaoServiceProfileRuntimeSpec `json:"runtime,omitempty"`
	// Observability defines the approved observability and telemetry posture.
	// +optional
	Observability *OpenBaoServiceProfileObservabilitySpec `json:"observability,omitempty"`
	// Network defines the approved network dependency posture.
	// +optional
	Network *OpenBaoServiceProfileNetworkSpec `json:"network,omitempty"`
	// Backup defines the approved backup posture.
	Backup OpenBaoServiceProfileBackupSpec `json:"backup"`
	// Lifecycle defines approved steady-state lifecycle policy.
	Lifecycle OpenBaoServiceProfileLifecycleSpec `json:"lifecycle"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.cluster.version"
// +kubebuilder:printcolumn:name="Security",type="string",JSONPath=".spec.cluster.securityProfile"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoServiceProfile is the immutable platform-owned service offering catalog object.
type OpenBaoServiceProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoServiceProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoServiceProfileList contains a list of OpenBaoServiceProfile.
type OpenBaoServiceProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoServiceProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoServiceProfile{}, &OpenBaoServiceProfileList{})
}
