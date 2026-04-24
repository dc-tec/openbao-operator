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

// OpenBaoUnsealProfileMode identifies a platform-owned unseal implementation posture.
// +kubebuilder:validation:Enum=OperatorManagedStatic;Transit;AWSKMS;GCPCloudKMS;AzureKeyVault;OCIKMS;KMIP;PKCS11
type OpenBaoUnsealProfileMode string

const (
	// OpenBaoUnsealProfileModeOperatorManagedStatic uses operator-managed static unseal.
	OpenBaoUnsealProfileModeOperatorManagedStatic OpenBaoUnsealProfileMode = "OperatorManagedStatic"
	// OpenBaoUnsealProfileModeTransit uses OpenBao transit seal.
	OpenBaoUnsealProfileModeTransit OpenBaoUnsealProfileMode = "Transit"
	// OpenBaoUnsealProfileModeAWSKMS uses AWS KMS seal.
	OpenBaoUnsealProfileModeAWSKMS OpenBaoUnsealProfileMode = "AWSKMS"
	// OpenBaoUnsealProfileModeGCPCloudKMS uses GCP Cloud KMS seal.
	OpenBaoUnsealProfileModeGCPCloudKMS OpenBaoUnsealProfileMode = "GCPCloudKMS"
	// OpenBaoUnsealProfileModeAzureKeyVault uses Azure Key Vault seal.
	OpenBaoUnsealProfileModeAzureKeyVault OpenBaoUnsealProfileMode = "AzureKeyVault"
	// OpenBaoUnsealProfileModeOCIKMS uses OCI KMS seal.
	OpenBaoUnsealProfileModeOCIKMS OpenBaoUnsealProfileMode = "OCIKMS"
	// OpenBaoUnsealProfileModeKMIP uses KMIP seal.
	OpenBaoUnsealProfileModeKMIP OpenBaoUnsealProfileMode = "KMIP"
	// OpenBaoUnsealProfileModePKCS11 uses PKCS#11 seal.
	OpenBaoUnsealProfileModePKCS11 OpenBaoUnsealProfileMode = "PKCS11"
)

// OpenBaoUnsealProfileSpec defines the desired state of OpenBaoUnsealProfile.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable after creation"
// +kubebuilder:validation:XValidation:rule="self.mode != 'Transit' || has(self.transit)",message="spec.transit is required when mode is Transit"
// +kubebuilder:validation:XValidation:rule="self.mode != 'AWSKMS' || has(self.awskms)",message="spec.awskms is required when mode is AWSKMS"
// +kubebuilder:validation:XValidation:rule="self.mode != 'GCPCloudKMS' || has(self.gcpCloudKMS)",message="spec.gcpCloudKMS is required when mode is GCPCloudKMS"
// +kubebuilder:validation:XValidation:rule="self.mode != 'AzureKeyVault' || has(self.azureKeyVault)",message="spec.azureKeyVault is required when mode is AzureKeyVault"
// +kubebuilder:validation:XValidation:rule="self.mode != 'OCIKMS' || has(self.ocikms)",message="spec.ocikms is required when mode is OCIKMS"
// +kubebuilder:validation:XValidation:rule="self.mode != 'KMIP' || has(self.kmip)",message="spec.kmip is required when mode is KMIP"
// +kubebuilder:validation:XValidation:rule="self.mode != 'PKCS11' || has(self.pkcs11)",message="spec.pkcs11 is required when mode is PKCS11"
type OpenBaoUnsealProfileSpec struct {
	// Mode identifies the unseal implementation.
	Mode OpenBaoUnsealProfileMode `json:"mode"`
	// Static configures the static seal type when Mode is OperatorManagedStatic.
	// +optional
	Static *StaticSealConfig `json:"static,omitempty"`
	// Transit configures the transit seal type when Mode is Transit.
	// +optional
	Transit *TransitSealConfig `json:"transit,omitempty"`
	// AWSKMS configures the AWS KMS seal type when Mode is AWSKMS.
	// +optional
	AWSKMS *AWSKMSSealConfig `json:"awskms,omitempty"`
	// AzureKeyVault configures the Azure Key Vault seal type when Mode is AzureKeyVault.
	// +optional
	AzureKeyVault *AzureKeyVaultSealConfig `json:"azureKeyVault,omitempty"`
	// GCPCloudKMS configures the GCP Cloud KMS seal type when Mode is GCPCloudKMS.
	// +optional
	GCPCloudKMS *GCPCloudKMSSealConfig `json:"gcpCloudKMS,omitempty"`
	// KMIP configures the KMIP seal type when Mode is KMIP.
	// +optional
	KMIP *KMIPSealConfig `json:"kmip,omitempty"`
	// OCIKMS configures the OCI KMS seal type when Mode is OCIKMS.
	// +optional
	OCIKMS *OCIKMSSealConfig `json:"ocikms,omitempty"`
	// PKCS11 configures the PKCS#11 seal type when Mode is PKCS11.
	// +optional
	PKCS11 *PKCS11SealConfig `json:"pkcs11,omitempty"`
	// CredentialsSecretRef references provider credentials in the target namespace.
	// Omit this when the profile relies on ambient workload identity.
	// +optional
	CredentialsSecretRef *corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Mode",type="string",JSONPath=".spec.mode"
// +kubebuilder:printcolumn:name="Credentials",type="string",JSONPath=".spec.credentialsSecretRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OpenBaoUnsealProfile is the immutable platform-owned unseal implementation catalog object.
type OpenBaoUnsealProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OpenBaoUnsealProfileSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// OpenBaoUnsealProfileList contains a list of OpenBaoUnsealProfile.
type OpenBaoUnsealProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenBaoUnsealProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenBaoUnsealProfile{}, &OpenBaoUnsealProfileList{})
}
