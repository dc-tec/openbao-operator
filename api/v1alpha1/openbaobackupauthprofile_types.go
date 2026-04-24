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

// OpenBaoBackupAuthMode identifies how backup executor jobs obtain storage credentials.
// +kubebuilder:validation:Enum=WorkloadIdentity;StaticCredentials
type OpenBaoBackupAuthMode string

const (
	// OpenBaoBackupAuthModeWorkloadIdentity uses workload identity or ambient default credentials.
	OpenBaoBackupAuthModeWorkloadIdentity OpenBaoBackupAuthMode = "WorkloadIdentity"
	// OpenBaoBackupAuthModeStaticCredentials uses a Secret in the materialized cluster namespace.
	OpenBaoBackupAuthModeStaticCredentials OpenBaoBackupAuthMode = "StaticCredentials"
)

// OpenBaoBackupStaticCredentialsSpec defines static credential posture for backup executor jobs.
type OpenBaoBackupStaticCredentialsSpec struct {
	// SecretName is the Secret name expected in the materialized cluster namespace.
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
}

// OpenBaoBackupAuthProfileSpec defines the desired state of OpenBaoBackupAuthProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
// +kubebuilder:validation:XValidation:rule="self.mode != 'StaticCredentials' || has(self.staticCredentials)",message="spec.staticCredentials is required when mode is StaticCredentials"
// +kubebuilder:validation:XValidation:rule="self.mode == 'StaticCredentials' || !has(self.staticCredentials)",message="spec.staticCredentials is only supported when mode is StaticCredentials"
type OpenBaoBackupAuthProfileSpec struct {
	// Mode identifies how backup executor jobs obtain storage credentials.
	Mode OpenBaoBackupAuthMode `json:"mode"`
	// StaticCredentials defines the Secret-backed credentials posture when required.
	// +optional
	StaticCredentials *OpenBaoBackupStaticCredentialsSpec `json:"staticCredentials,omitempty"`
	// WorkloadIdentity defines provider-specific metadata for workload-identity-backed storage access.
	// +optional
	WorkloadIdentity *WorkloadIdentityConfig `json:"workloadIdentity,omitempty"`
	// RoleARN identifies an explicit web-identity role assumption target when the provider requires it.
	// +optional
	RoleARN string `json:"roleArn,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode"
// +kubebuilder:printcolumn:name="Secret",type="string",JSONPath=".spec.staticCredentials.secretName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoBackupAuthProfile is the immutable platform-owned backup auth posture object.
type OpenBaoBackupAuthProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoBackupAuthProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoBackupAuthProfileList contains a list of OpenBaoBackupAuthProfile.
type OpenBaoBackupAuthProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoBackupAuthProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoBackupAuthProfile{}, &OpenBaoBackupAuthProfileList{})
}
