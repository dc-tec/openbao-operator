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

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

type catalogObjectBinding struct {
	ref             *openbaov1alpha1.LocalReference
	object          metav1.Object
	requiredMessage string
	mismatchMessage string
}

func validateApprovedCatalogInputs(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	catalog *CatalogBundle,
) ValidationResult {
	if claim == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim is required to bind an approved service contract.",
		}
	}
	if catalog == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Immutable catalog inputs are required to bind an approved service contract.",
		}
	}
	serviceProfile := catalog.ServiceProfile
	if serviceProfile == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoServiceProfile is required to bind an approved service contract.",
		}
	}
	if serviceProfile.Name != claim.Spec.ServiceProfileRef.Name {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Loaded OpenBaoServiceProfile does not match OpenBaoClusterClaim.spec.serviceProfileRef.",
		}
	}

	for _, binding := range approvedCatalogObjectBindings(serviceProfile, catalog) {
		if validation := validateCatalogObjectBinding(binding); !validation.Valid {
			return validation
		}
	}

	return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func approvedCatalogObjectBindings(
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	catalog *CatalogBundle,
) []catalogObjectBinding {
	var unsealRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Unseal != nil {
		unsealRef = serviceProfile.Spec.Unseal.ProfileRef
	}
	var runtimeRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Runtime != nil {
		runtimeRef = serviceProfile.Spec.Runtime.ProfileRef
	}
	var observabilityRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Observability != nil {
		observabilityRef = serviceProfile.Spec.Observability.ProfileRef
	}
	var networkRef *openbaov1alpha1.LocalReference
	if serviceProfile.Spec.Network != nil {
		networkRef = serviceProfile.Spec.Network.ProfileRef
	}

	return []catalogObjectBinding{
		{
			ref:             &serviceProfile.Spec.Exposure.ClassRef,
			object:          catalog.ExposureClass,
			requiredMessage: "OpenBaoExposureClass is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoExposureClass does not match OpenBaoServiceProfile.spec.exposure.classRef.",
		},
		{
			ref:             &serviceProfile.Spec.Backup.ProfileRef,
			object:          catalog.BackupProfile,
			requiredMessage: "OpenBaoBackupProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoBackupProfile does not match OpenBaoServiceProfile.spec.backup.profileRef.",
		},
		{
			ref:             serviceProfile.Spec.Bootstrap.ProfileRef,
			object:          catalog.BootstrapProfile,
			requiredMessage: "OpenBaoBootstrapProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoBootstrapProfile does not match OpenBaoServiceProfile.spec.bootstrap.profileRef.",
		},
		{
			ref:             serviceProfile.Spec.Storage.ProfileRef,
			object:          catalog.StorageProfile,
			requiredMessage: "OpenBaoStorageProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoStorageProfile does not match OpenBaoServiceProfile.spec.storage.profileRef.",
		},
		{
			ref:             unsealRef,
			object:          catalog.UnsealProfile,
			requiredMessage: "OpenBaoUnsealProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoUnsealProfile does not match OpenBaoServiceProfile.spec.unseal.profileRef.",
		},
		{
			ref:             runtimeRef,
			object:          catalog.RuntimeProfile,
			requiredMessage: "OpenBaoRuntimeProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoRuntimeProfile does not match OpenBaoServiceProfile.spec.runtime.profileRef.",
		},
		{
			ref:             observabilityRef,
			object:          catalog.ObservabilityProfile,
			requiredMessage: "OpenBaoObservabilityProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoObservabilityProfile does not match OpenBaoServiceProfile.spec.observability.profileRef.",
		},
		{
			ref:             networkRef,
			object:          catalog.NetworkProfile,
			requiredMessage: "OpenBaoNetworkProfile is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoNetworkProfile does not match OpenBaoServiceProfile.spec.network.profileRef.",
		},
		{
			ref:             serviceProfile.Spec.Lifecycle.PolicyRef,
			object:          catalog.UpgradePolicy,
			requiredMessage: "OpenBaoUpgradePolicy is required to bind an approved service contract.",
			mismatchMessage: "Loaded OpenBaoUpgradePolicy does not match OpenBaoServiceProfile.spec.lifecycle.policyRef.",
		},
	}
}

func validateCatalogObjectBinding(binding catalogObjectBinding) ValidationResult {
	if binding.ref == nil {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if isNilCatalogObject(binding.object) {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: binding.requiredMessage,
		}
	}
	if binding.object.GetName() != binding.ref.Name {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: binding.mismatchMessage,
		}
	}
	return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func isNilCatalogObject(object metav1.Object) bool {
	if object == nil {
		return true
	}
	value := reflect.ValueOf(object)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

// AppliedStatus projects the bound top-level catalog provenance onto claim status.
func AppliedStatus(contract *ApprovedServiceContract) openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	if contract == nil {
		return openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{}
	}

	return openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
		ServiceProfileRef:       boundRevisionCopy(&contract.Provenance.ServiceProfileRef),
		BootstrapProfileRef:     boundRevisionCopy(contract.Provenance.BootstrapProfileRef),
		ExposureClassRef:        boundRevisionCopy(contract.Provenance.ExposureClassRef),
		StorageProfileRef:       boundRevisionCopy(contract.Provenance.StorageProfileRef),
		UnsealProfileRef:        boundRevisionCopy(contract.Provenance.UnsealProfileRef),
		RuntimeProfileRef:       boundRevisionCopy(contract.Provenance.RuntimeProfileRef),
		ObservabilityProfileRef: boundRevisionCopy(contract.Provenance.ObservabilityProfileRef),
		NetworkProfileRef:       boundRevisionCopy(contract.Provenance.NetworkProfileRef),
		UpgradePolicyRef:        boundRevisionCopy(contract.Provenance.UpgradePolicyRef),
		BackupProfileRef:        boundRevisionCopy(contract.Provenance.BackupProfileRef),
	}
}

// ValidateContinuity fails closed if a previously applied immutable revision name now resolves to a different UID.
func ValidateContinuity(applied openbaov1alpha1.OpenBaoClusterClaimAppliedStatus, contract *ApprovedServiceContract) ValidationResult {
	if contract == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Approved service contract is required to validate continuity.",
		}
	}

	checks := []struct {
		display string
		applied *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
		desired *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	}{
		{display: "OpenBaoServiceProfile", applied: applied.ServiceProfileRef, desired: &contract.Provenance.ServiceProfileRef},
		{display: "OpenBaoBootstrapProfile", applied: applied.BootstrapProfileRef, desired: contract.Provenance.BootstrapProfileRef},
		{display: "OpenBaoExposureClass", applied: applied.ExposureClassRef, desired: contract.Provenance.ExposureClassRef},
		{display: "OpenBaoStorageProfile", applied: applied.StorageProfileRef, desired: contract.Provenance.StorageProfileRef},
		{display: "OpenBaoUnsealProfile", applied: applied.UnsealProfileRef, desired: contract.Provenance.UnsealProfileRef},
		{display: "OpenBaoRuntimeProfile", applied: applied.RuntimeProfileRef, desired: contract.Provenance.RuntimeProfileRef},
		{display: "OpenBaoObservabilityProfile", applied: applied.ObservabilityProfileRef, desired: contract.Provenance.ObservabilityProfileRef},
		{display: "OpenBaoNetworkProfile", applied: applied.NetworkProfileRef, desired: contract.Provenance.NetworkProfileRef},
		{display: "OpenBaoUpgradePolicy", applied: applied.UpgradePolicyRef, desired: contract.Provenance.UpgradePolicyRef},
		{display: "OpenBaoBackupProfile", applied: applied.BackupProfileRef, desired: contract.Provenance.BackupProfileRef},
	}
	for _, check := range checks {
		if result := validateBoundRevisionContinuity(check.display, check.applied, check.desired); !result.Valid {
			return result
		}
	}

	return ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Approved service contract continuity matches previously applied immutable revisions.",
	}
}

func validateBoundRevisionContinuity(
	display string,
	applied *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference,
	desired *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference,
) ValidationResult {
	if applied == nil || applied.Name == "" || applied.UID == "" {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if desired == nil || desired.Name == "" || desired.UID == "" {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if applied.Name != desired.Name {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	if applied.UID == desired.UID {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}

	return ValidationResult{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonInvalid,
		Message: display + " continuity is invalid because the bound immutable revision name now resolves to a different UID.",
	}
}

func boundRevisionReference(object metav1.Object) openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	return openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
		Name: object.GetName(),
		UID:  string(object.GetUID()),
	}
}

func boundRevisionReferencePtr(object metav1.Object) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if object == nil {
		return nil
	}

	ref := boundRevisionReference(object)
	return &ref
}

func boundRevisionCopy(ref *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if ref == nil {
		return nil
	}

	copy := *ref
	return &copy
}
