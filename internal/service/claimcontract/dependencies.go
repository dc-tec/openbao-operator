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

// AppliedRenderedDependencies projects immutable lower execution-policy
// dependency identities from one rendered same-cluster contract onto claim status.
func AppliedRenderedDependencies(rendered *RenderedExecutionContract) *openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus {
	if rendered == nil {
		return nil
	}

	status := &openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus{
		EntrypointRef:           boundRenderedReferenceCopy(renderedExposureEntrypointRef(rendered)),
		IngressPolicyRef:        boundRenderedReferenceCopy(renderedExposureIngressPolicyRef(rendered)),
		BackupTargetRef:         boundRenderedReferenceCopy(rendered.Backup.TargetRef),
		BackupBackendRef:        boundRenderedReferenceCopy(rendered.Backup.BackendRef),
		BackupAuthProfileRef:    boundRenderedReferenceCopy(rendered.Backup.AuthProfileRef),
		TransferProfileRef:      boundRenderedReferenceCopy(rendered.Backup.TransferProfileRef),
		BootstrapProjectionRefs: renderedBootstrapProjectionRefs(rendered),
	}
	status.BootstrapProjectionIdentity = renderedBootstrapProjectionIdentity(rendered)
	status.Identity = ContractIdentityStatus(IdentityHash(statusWithoutIdentity(status)))
	if status.EntrypointRef == nil &&
		status.IngressPolicyRef == nil &&
		status.BackupTargetRef == nil &&
		status.BackupBackendRef == nil &&
		status.BackupAuthProfileRef == nil &&
		status.TransferProfileRef == nil &&
		status.BootstrapProjectionIdentity == nil &&
		len(status.BootstrapProjectionRefs) == 0 {
		return nil
	}
	return status
}

// ValidateRenderedDependencyContinuity fails closed if a previously applied
// immutable lower execution-policy dependency name now resolves to a different UID.
func ValidateRenderedDependencyContinuity(
	applied openbaov1alpha1.OpenBaoClusterClaimAppliedStatus,
	rendered *RenderedExecutionContract,
) ValidationResult {
	if rendered == nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to validate dependency continuity.",
		}
	}

	desired := AppliedRenderedDependencies(rendered)
	if desired == nil {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}

	current := applied.RenderedDependencies
	if current == nil {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}

	checks := []struct {
		display string
		applied *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
		desired *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference
	}{
		{display: "OpenBaoEntrypoint", applied: current.EntrypointRef, desired: desired.EntrypointRef},
		{display: "OpenBaoIngressPolicy", applied: current.IngressPolicyRef, desired: desired.IngressPolicyRef},
		{display: "OpenBaoBackupTarget", applied: current.BackupTargetRef, desired: desired.BackupTargetRef},
		{display: "OpenBaoBackupBackend", applied: current.BackupBackendRef, desired: desired.BackupBackendRef},
		{display: "OpenBaoBackupAuthProfile", applied: current.BackupAuthProfileRef, desired: desired.BackupAuthProfileRef},
		{display: "OpenBaoTransferProfile", applied: current.TransferProfileRef, desired: desired.TransferProfileRef},
	}
	for _, check := range checks {
		if result := validateBoundRevisionContinuity(check.display, check.applied, check.desired); !result.Valid {
			return result
		}
	}
	if current.BootstrapProjectionIdentity != nil &&
		desired.BootstrapProjectionIdentity != nil &&
		current.BootstrapProjectionIdentity.IdentityHash != desired.BootstrapProjectionIdentity.IdentityHash {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Bootstrap projected dependency continuity is invalid because the rendered bootstrap dependency artifact set changed after materialization.",
		}
	}

	return ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered execution dependency continuity matches previously applied immutable revisions.",
	}
}

func renderedExposureEntrypointRef(rendered *RenderedExecutionContract) *RenderedBoundReference {
	if rendered == nil || rendered.Exposure.Entrypoint == nil {
		return nil
	}
	return rendered.Exposure.Entrypoint.Ref
}

func renderedExposureIngressPolicyRef(rendered *RenderedExecutionContract) *RenderedBoundReference {
	if rendered == nil || rendered.Exposure.Ingress == nil {
		return nil
	}
	return rendered.Exposure.Ingress.PolicyRef
}

func boundRenderedReferenceCopy(ref *RenderedBoundReference) *openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference {
	if ref == nil || ref.Name == "" || ref.UID == "" {
		return nil
	}
	return &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: ref.Name, UID: ref.UID}
}

func statusWithoutIdentity(status *openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus) openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus {
	if status == nil {
		return openbaov1alpha1.OpenBaoClusterClaimRenderedDependencyStatus{}
	}
	copy := *status
	copy.Identity = nil
	return copy
}

type bootstrapProjectionIdentityPayload struct {
	AuthMethodConfigRefs []openbaov1alpha1.TypedObjectReference `json:"authMethodConfigRefs,omitempty"`
	PolicyContentRefs    []openbaov1alpha1.TypedObjectReference `json:"policyContentRefs,omitempty"`
	AuditSinkRefs        []openbaov1alpha1.TypedObjectReference `json:"auditSinkRefs,omitempty"`
}

func renderedBootstrapProjectionRefs(rendered *RenderedExecutionContract) []openbaov1alpha1.TypedObjectReference {
	if rendered == nil {
		return nil
	}

	refs := make([]openbaov1alpha1.TypedObjectReference, 0)
	if rendered.Bootstrap.Auth != nil {
		for _, method := range rendered.Bootstrap.Auth.Methods {
			if method.ConfigFromRef != nil && method.ConfigFromRef.Name != "" && method.ConfigFromRef.Kind != "" {
				refs = append(refs, *cloneTypedObjectReference(method.ConfigFromRef))
			}
		}
	}
	if rendered.Bootstrap.Policies != nil {
		for _, bundle := range rendered.Bootstrap.Policies.Bundles {
			if bundle.ContentFromRef.Name != "" && bundle.ContentFromRef.Kind != "" {
				refs = append(refs, bundle.ContentFromRef)
			}
		}
	}
	if rendered.Bootstrap.Audit != nil {
		for _, device := range rendered.Bootstrap.Audit.Devices {
			if device.SinkFromRef != nil && device.SinkFromRef.Name != "" && device.SinkFromRef.Kind != "" {
				refs = append(refs, *cloneTypedObjectReference(device.SinkFromRef))
			}
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func renderedBootstrapProjectionIdentity(rendered *RenderedExecutionContract) *openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus {
	if rendered == nil {
		return nil
	}

	payload := bootstrapProjectionIdentityPayload{}
	if rendered.Bootstrap.Auth != nil {
		for _, method := range rendered.Bootstrap.Auth.Methods {
			if method.ConfigFromRef != nil && method.ConfigFromRef.Name != "" && method.ConfigFromRef.Kind != "" {
				payload.AuthMethodConfigRefs = append(payload.AuthMethodConfigRefs, *cloneTypedObjectReference(method.ConfigFromRef))
			}
		}
	}
	if rendered.Bootstrap.Policies != nil {
		for _, bundle := range rendered.Bootstrap.Policies.Bundles {
			if bundle.ContentFromRef.Name != "" && bundle.ContentFromRef.Kind != "" {
				payload.PolicyContentRefs = append(payload.PolicyContentRefs, bundle.ContentFromRef)
			}
		}
	}
	if rendered.Bootstrap.Audit != nil {
		for _, device := range rendered.Bootstrap.Audit.Devices {
			if device.SinkFromRef != nil && device.SinkFromRef.Name != "" && device.SinkFromRef.Kind != "" {
				payload.AuditSinkRefs = append(payload.AuditSinkRefs, *cloneTypedObjectReference(device.SinkFromRef))
			}
		}
	}

	if len(payload.AuthMethodConfigRefs) == 0 &&
		len(payload.PolicyContentRefs) == 0 &&
		len(payload.AuditSinkRefs) == 0 {
		return nil
	}

	return ContractIdentityStatus(IdentityHash(payload))
}
