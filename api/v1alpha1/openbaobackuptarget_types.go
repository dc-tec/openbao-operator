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

// OpenBaoBackupLocationMode identifies how the top-level backup location is selected.
// +kubebuilder:validation:Enum=Fixed;Template;ClaimValue
type OpenBaoBackupLocationMode string

const (
	// OpenBaoBackupLocationModeFixed uses one fixed location value for every claim.
	OpenBaoBackupLocationModeFixed OpenBaoBackupLocationMode = "Fixed"
	// OpenBaoBackupLocationModeTemplate derives the location from approved claim context.
	OpenBaoBackupLocationModeTemplate OpenBaoBackupLocationMode = "Template"
	// OpenBaoBackupLocationModeClaimValue allows a bounded claim-provided location value.
	OpenBaoBackupLocationModeClaimValue OpenBaoBackupLocationMode = "ClaimValue"
)

// OpenBaoBackupDeletionBehavior identifies how external backups are treated when the owning service is removed.
// +kubebuilder:validation:Enum=Retain;DeleteWithService
type OpenBaoBackupDeletionBehavior string

const (
	// OpenBaoBackupDeletionBehaviorRetain keeps external backups when the service is removed.
	OpenBaoBackupDeletionBehaviorRetain OpenBaoBackupDeletionBehavior = "Retain"
	// OpenBaoBackupDeletionBehaviorDeleteWithService allows service removal to request external backup deletion.
	OpenBaoBackupDeletionBehaviorDeleteWithService OpenBaoBackupDeletionBehavior = "DeleteWithService"
)

// OpenBaoBackupLocationSelectionSpec defines how the top-level backup location is derived.
type OpenBaoBackupLocationSelectionSpec struct {
	// Mode identifies how the location is selected.
	Mode OpenBaoBackupLocationMode `json:"mode"`
	// Value is the fixed location when Mode is Fixed.
	// +optional
	Value string `json:"value,omitempty"`
	// Template is the deterministic location template when Mode is Template.
	// +optional
	Template string `json:"template,omitempty"`
	// ValidationPattern constrains claim-provided or rendered location values when set.
	// +optional
	ValidationPattern string `json:"validationPattern,omitempty"`
}

// OpenBaoBackupKeyPrefixPolicySpec defines deterministic backup key-prefix posture.
type OpenBaoBackupKeyPrefixPolicySpec struct {
	// Template is the deterministic rendered key-prefix template.
	// +kubebuilder:validation:MinLength=1
	Template string `json:"template"`
	// AllowClaimPartition identifies whether claim.serviceParameters.backup.partition may influence the rendered prefix.
	// +optional
	AllowClaimPartition bool `json:"allowClaimPartition,omitempty"`
}

// OpenBaoBackupLocationPolicySpec defines where backups are allowed to land and how keys are laid out.
type OpenBaoBackupLocationPolicySpec struct {
	// Location defines how the top-level backup location is selected.
	Location OpenBaoBackupLocationSelectionSpec `json:"location"`
	// KeyPrefix defines deterministic key-prefix posture under the selected location.
	KeyPrefix OpenBaoBackupKeyPrefixPolicySpec `json:"keyPrefix"`
}

// OpenBaoBackupTargetPolicySpec defines deletion posture for external backup data.
type OpenBaoBackupTargetPolicySpec struct {
	// DeletionBehavior identifies how external backups are treated when the owning service is removed.
	// +kubebuilder:default=Retain
	// +optional
	DeletionBehavior OpenBaoBackupDeletionBehavior `json:"deletionBehavior,omitempty"`
}

// OpenBaoBackupTargetSpec defines the desired state of OpenBaoBackupTarget.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
type OpenBaoBackupTargetSpec struct {
	// BackendRef identifies the immutable concrete backup backend revision.
	BackendRef LocalReference `json:"backendRef"`
	// AuthProfileRef identifies the immutable concrete backup auth profile revision when required.
	// +optional
	AuthProfileRef *LocalReference `json:"authProfileRef,omitempty"`
	// TransportProfileRef identifies the immutable upload/transfer tuning profile when required.
	// +optional
	TransportProfileRef *LocalReference `json:"transportProfileRef,omitempty"`
	// LocationPolicy defines where backups are allowed to land and how keys are laid out.
	LocationPolicy OpenBaoBackupLocationPolicySpec `json:"locationPolicy"`
	// Policy defines service-removal posture for external backup data.
	// +optional
	Policy *OpenBaoBackupTargetPolicySpec `json:"policy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Backend",type="string",JSONPath=".spec.backendRef.name"
// +kubebuilder:printcolumn:name="Auth",type="string",JSONPath=".spec.authProfileRef.name"
// +kubebuilder:printcolumn:name="Transfer",type="string",JSONPath=".spec.transportProfileRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoBackupTarget is the immutable platform-owned backup destination policy object.
type OpenBaoBackupTarget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoBackupTargetSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoBackupTargetList contains a list of OpenBaoBackupTarget.
type OpenBaoBackupTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoBackupTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoBackupTarget{}, &OpenBaoBackupTargetList{})
}
