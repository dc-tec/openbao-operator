// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

const (
	unsealTypeStatic        = "static"
	unsealTypeTransit       = "transit"
	unsealTypeAWSKMS        = "awskms"
	unsealTypeGCPCloudKMS   = "gcpckms"
	unsealTypeAzureKeyVault = "azurekeyvault"
	unsealTypeOCIKMS        = "ocikms"
	unsealTypeKMIP          = "kmip"
	unsealTypePKCS11        = "pkcs11"
)

func bindApprovedUnseal(
	securityProfile openbaov1alpha1.Profile,
	unsealProfile *openbaov1alpha1.OpenBaoUnsealProfile,
) (ApprovedUnseal, ValidationResult) {
	if unsealProfile == nil {
		return ApprovedUnseal{Mode: approvedUnsealMode(securityProfile)}, ValidationResult{
			Valid:  true,
			Reason: openbaov1alpha1.ReasonAccepted,
		}
	}

	mode := approvedUnsealModeFromProfile(unsealProfile.Spec.Mode)
	if securityProfile == openbaov1alpha1.ProfileHardened && mode == UnsealPostureModeManagedStatic {
		return ApprovedUnseal{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Hardened service profiles require a non-static OpenBaoUnsealProfile.",
		}
	}

	config, validation := unsealConfigFromProfile(unsealProfile)
	if !validation.Valid {
		return ApprovedUnseal{}, validation
	}
	return ApprovedUnseal{
			Mode:       mode,
			ProfileRef: &openbaov1alpha1.LocalReference{Name: unsealProfile.Name},
			Config:     config,
		}, ValidationResult{
			Valid:  true,
			Reason: openbaov1alpha1.ReasonAccepted,
		}
}

func approvedUnsealModeFromProfile(mode openbaov1alpha1.OpenBaoUnsealProfileMode) UnsealPostureMode {
	if mode == "" || mode == openbaov1alpha1.OpenBaoUnsealProfileModeOperatorManagedStatic {
		return UnsealPostureModeManagedStatic
	}
	return UnsealPostureModeExternal
}

func unsealConfigFromProfile(profile *openbaov1alpha1.OpenBaoUnsealProfile) (*openbaov1alpha1.UnsealConfig, ValidationResult) {
	if profile == nil {
		return nil, ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}

	spec := profile.Spec
	config := &openbaov1alpha1.UnsealConfig{
		CredentialsSecretRef: cloneLocalObjectReference(spec.CredentialsSecretRef),
	}
	switch spec.Mode {
	case "", openbaov1alpha1.OpenBaoUnsealProfileModeOperatorManagedStatic:
		config.Type = unsealTypeStatic
		config.Static = cloneStaticSealConfig(spec.Static)
	case openbaov1alpha1.OpenBaoUnsealProfileModeTransit:
		if spec.Transit == nil {
			return nil, missingUnsealProfileSection(profile.Name, "transit")
		}
		config.Type = unsealTypeTransit
		config.Transit = spec.Transit.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeAWSKMS:
		if spec.AWSKMS == nil {
			return nil, missingUnsealProfileSection(profile.Name, "awskms")
		}
		config.Type = unsealTypeAWSKMS
		config.AWSKMS = spec.AWSKMS.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeGCPCloudKMS:
		if spec.GCPCloudKMS == nil {
			return nil, missingUnsealProfileSection(profile.Name, "gcpCloudKMS")
		}
		config.Type = unsealTypeGCPCloudKMS
		config.GCPCloudKMS = spec.GCPCloudKMS.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeAzureKeyVault:
		if spec.AzureKeyVault == nil {
			return nil, missingUnsealProfileSection(profile.Name, "azureKeyVault")
		}
		config.Type = unsealTypeAzureKeyVault
		config.AzureKeyVault = spec.AzureKeyVault.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeOCIKMS:
		if spec.OCIKMS == nil {
			return nil, missingUnsealProfileSection(profile.Name, "ocikms")
		}
		config.Type = unsealTypeOCIKMS
		config.OCIKMS = spec.OCIKMS.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModeKMIP:
		if spec.KMIP == nil {
			return nil, missingUnsealProfileSection(profile.Name, "kmip")
		}
		config.Type = unsealTypeKMIP
		config.KMIP = spec.KMIP.DeepCopy()
	case openbaov1alpha1.OpenBaoUnsealProfileModePKCS11:
		if spec.PKCS11 == nil {
			return nil, missingUnsealProfileSection(profile.Name, "pkcs11")
		}
		config.Type = unsealTypePKCS11
		config.PKCS11 = spec.PKCS11.DeepCopy()
	default:
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoUnsealProfile uses an unsupported mode.",
		}
	}

	if config.Type == unsealTypeStatic && config.Static == nil && config.CredentialsSecretRef == nil {
		return nil, ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	return config, ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func missingUnsealProfileSection(profileName, section string) ValidationResult {
	return ValidationResult{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: "OpenBaoUnsealProfile " + profileName + " is missing required spec." + section + " configuration.",
	}
}

func approvedUnsealMode(profile openbaov1alpha1.Profile) UnsealPostureMode {
	if profile == openbaov1alpha1.ProfileHardened {
		return UnsealPostureModeExternal
	}

	return UnsealPostureModeManagedStatic
}
