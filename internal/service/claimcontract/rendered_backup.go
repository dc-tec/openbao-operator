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
	"regexp"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func renderBackup(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.NamespacedReference,
	approved *ApprovedServiceContract,
	catalog *CatalogBundle,
) (RenderedBackup, ValidationResult) {
	rendered := RenderedBackup{
		Schedule:   catalog.BackupProfile.Spec.Schedule,
		Retention:  cloneRetention(catalog.BackupProfile.Spec.Retention),
		Partition:  approved.Backup.Parameters.Partition,
		ProfileRef: approved.Backup.ProfileRef,
	}

	if catalog.BackupProfile.Spec.TargetRef == nil {
		return rendered, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract does not require concrete backup target inputs.",
		}
	}
	if catalog.BackupTarget == nil {
		return RenderedBackup{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoBackupTarget is required to render concrete backup execution inputs.",
		}
	}
	if catalog.BackupTarget.Name != catalog.BackupProfile.Spec.TargetRef.Name {
		return RenderedBackup{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Loaded OpenBaoBackupTarget does not match OpenBaoBackupProfile.spec.targetRef.",
		}
	}
	if catalog.BackupBackend == nil {
		return RenderedBackup{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoBackupBackend is required to render concrete backup execution inputs.",
		}
	}
	if catalog.BackupBackend.Name != catalog.BackupTarget.Spec.BackendRef.Name {
		return RenderedBackup{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Loaded OpenBaoBackupBackend does not match OpenBaoBackupTarget.spec.backendRef.",
		}
	}
	if catalog.BackupTarget.Spec.AuthProfileRef != nil {
		if catalog.BackupAuth == nil {
			return RenderedBackup{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "OpenBaoBackupAuthProfile is required to render concrete backup auth inputs.",
			}
		}
		if catalog.BackupAuth.Name != catalog.BackupTarget.Spec.AuthProfileRef.Name {
			return RenderedBackup{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Loaded OpenBaoBackupAuthProfile does not match OpenBaoBackupTarget.spec.authProfileRef.",
			}
		}
	}
	if catalog.BackupTarget.Spec.TransportProfileRef != nil {
		if catalog.TransferProfile == nil {
			return RenderedBackup{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "OpenBaoTransferProfile is required to render concrete backup transfer inputs.",
			}
		}
		if catalog.TransferProfile.Name != catalog.BackupTarget.Spec.TransportProfileRef.Name {
			return RenderedBackup{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Loaded OpenBaoTransferProfile does not match OpenBaoBackupTarget.spec.transportProfileRef.",
			}
		}
	}

	location, validation := renderedBackupLocation(claim, target, approved, catalog.BackupTarget)
	if !validation.Valid {
		return RenderedBackup{}, validation
	}
	keyPrefix, validation := renderedBackupKeyPrefix(claim, target, approved, catalog.BackupTarget)
	if !validation.Valid {
		return RenderedBackup{}, validation
	}
	backend, validation := renderedBackupBackend(catalog.BackupBackend)
	if !validation.Valid {
		return RenderedBackup{}, validation
	}
	auth, validation := renderedBackupAuth(catalog.BackupAuth)
	if !validation.Valid {
		return RenderedBackup{}, validation
	}
	transfer, validation := renderedBackupTransfer(catalog.TransferProfile)
	if !validation.Valid {
		return RenderedBackup{}, validation
	}

	rendered.TargetRef = boundReference(catalog.BackupTarget.Name, string(catalog.BackupTarget.UID))
	rendered.BackendRef = boundReference(catalog.BackupBackend.Name, string(catalog.BackupBackend.UID))
	rendered.AuthProfileRef = boundReferenceFromAuthProfile(catalog.BackupAuth)
	rendered.TransferProfileRef = boundReferenceFromTransferProfile(catalog.TransferProfile)
	rendered.Location = location
	rendered.KeyPrefix = keyPrefix
	rendered.Backend = backend
	rendered.Auth = auth
	rendered.Transfer = transfer

	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered backup execution inputs have been produced from immutable backup implementation objects.",
	}
}

func renderedRequiredEgressRules(renderedBackup RenderedBackup) []networkingv1.NetworkPolicyEgressRule {
	if renderedBackup.Backend == nil {
		return nil
	}

	return cloneEgressRules(renderedBackup.Backend.RequiredEgressRules)
}

func renderedBackupLocation(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.NamespacedReference,
	approved *ApprovedServiceContract,
	backupTarget *openbaov1alpha1.OpenBaoBackupTarget,
) (string, ValidationResult) {
	if backupTarget == nil {
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoBackupTarget is required to render backup location.",
		}
	}

	selection := backupTarget.Spec.LocationPolicy.Location
	var value string
	switch selection.Mode {
	case openbaov1alpha1.OpenBaoBackupLocationModeFixed:
		value = strings.TrimSpace(selection.Value)
		if value == "" {
			return "", ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoBackupTarget fixed location policy requires a non-empty value.",
			}
		}
	case openbaov1alpha1.OpenBaoBackupLocationModeTemplate:
		if strings.TrimSpace(selection.Template) == "" {
			return "", ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoBackupTarget template location policy requires a non-empty template.",
			}
		}
		value = renderBackupTemplate(selection.Template, claim, target, approved)
		if strings.Contains(value, "{{") || strings.Contains(value, "}}") {
			return "", ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoBackupTarget location template contains unsupported placeholders.",
			}
		}
	case openbaov1alpha1.OpenBaoBackupLocationModeClaimValue:
		value = strings.TrimSpace(approved.Backup.Parameters.Location)
		if value == "" {
			return "", ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Claim-provided backup location is required by the selected OpenBaoBackupTarget policy.",
			}
		}
	default:
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoBackupTarget uses an unsupported backup location mode.",
		}
	}

	if validation := validateBackupLocation(value, selection.ValidationPattern); !validation.Valid {
		return "", validation
	}

	return value, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered backup location satisfies the selected OpenBaoBackupTarget policy.",
	}
}

func renderedBackupKeyPrefix(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.NamespacedReference,
	approved *ApprovedServiceContract,
	backupTarget *openbaov1alpha1.OpenBaoBackupTarget,
) (string, ValidationResult) {
	if backupTarget == nil {
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoBackupTarget is required to render backup key prefix.",
		}
	}

	base := renderBackupTemplate(backupTarget.Spec.LocationPolicy.KeyPrefix.Template, claim, target, approved)
	if strings.Contains(base, "{{") || strings.Contains(base, "}}") {
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoBackupTarget key-prefix template contains unsupported placeholders.",
		}
	}
	base = strings.Trim(base, "/")
	if base == "" {
		return "", ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoBackupTarget key-prefix template must render a non-empty prefix.",
		}
	}

	partition := strings.TrimSpace(approved.Backup.Parameters.Partition)
	if partition != "" {
		if !backupTarget.Spec.LocationPolicy.KeyPrefix.AllowClaimPartition {
			return "", ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Claim-provided backup partition is not allowed by the selected OpenBaoBackupTarget policy.",
			}
		}
		base = strings.Trim(base+"/"+partition, "/")
	}

	return base, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered backup key prefix satisfies the selected OpenBaoBackupTarget policy.",
	}
}

func validateBackupLocation(value, pattern string) ValidationResult {
	if strings.TrimSpace(pattern) == "" {
		return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
	}
	matched, err := regexp.MatchString(pattern, value)
	if err != nil {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoBackupTarget location validation pattern is invalid.",
		}
	}
	if !matched {
		return ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered backup location does not satisfy the selected OpenBaoBackupTarget validation pattern.",
		}
	}
	return ValidationResult{Valid: true, Reason: openbaov1alpha1.ReasonAccepted}
}

func renderBackupTemplate(
	template string,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	target *openbaov1alpha1.NamespacedReference,
	approved *ApprovedServiceContract,
) string {
	rendered := template
	replacements := map[string]string{
		"{{ tenant.name }}":     claim.Spec.TenantRef.Name,
		"{{ claim.namespace }}": claim.Namespace,
		"{{ claim.name }}":      claim.Name,
	}
	if target != nil {
		replacements["{{ target.namespace }}"] = target.Namespace
		replacements["{{ target.name }}"] = target.Name
	}
	if approved != nil {
		replacements["{{ backup.partition }}"] = strings.TrimSpace(approved.Backup.Parameters.Partition)
	}
	for placeholder, value := range replacements {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	return rendered
}

func renderedBackupBackend(
	backend *openbaov1alpha1.OpenBaoBackupBackend,
) (*RenderedBackupBackend, ValidationResult) {
	if backend == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoBackupBackend is required to render concrete backup backend inputs.",
		}
	}
	if backend.Spec.Driver != openbaov1alpha1.OpenBaoBackupBackendDriverObjectStorage || backend.Spec.ObjectStorage == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Only ObjectStorage-backed OpenBaoBackupBackend definitions are supported for same-cluster rendered backup inputs.",
		}
	}
	return &RenderedBackupBackend{
			Driver:              backend.Spec.Driver,
			Provider:            backend.Spec.ObjectStorage.Provider,
			Endpoint:            backend.Spec.ObjectStorage.Endpoint,
			Region:              backend.Spec.ObjectStorage.Region,
			UsePathStyle:        backend.Spec.ObjectStorage.UsePathStyle,
			GCSProject:          backend.Spec.ObjectStorage.GCSProject,
			AzureStorageAccount: backend.Spec.ObjectStorage.AzureStorageAccount,
			AzureContainer:      backend.Spec.ObjectStorage.AzureContainer,
			InsecureSkipVerify:  backend.Spec.ObjectStorage.InsecureSkipVerify,
			RequiredEgressRules: cloneEgressRules(backend.Spec.ObjectStorage.RequiredEgressRules),
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered backup backend inputs have been resolved.",
		}
}

func renderedBackupAuth(
	authProfile *openbaov1alpha1.OpenBaoBackupAuthProfile,
) (*RenderedBackupAuth, ValidationResult) {
	if authProfile == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered backup contract does not require an explicit backup auth profile.",
		}
	}
	rendered := &RenderedBackupAuth{
		Mode:             authProfile.Spec.Mode,
		WorkloadIdentity: cloneWorkloadIdentityConfig(authProfile.Spec.WorkloadIdentity),
		RoleARN:          authProfile.Spec.RoleARN,
	}
	if authProfile.Spec.Mode == openbaov1alpha1.OpenBaoBackupAuthModeStaticCredentials {
		if authProfile.Spec.StaticCredentials == nil || strings.TrimSpace(authProfile.Spec.StaticCredentials.SecretName) == "" {
			return nil, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoBackupAuthProfile static-credentials mode requires a non-empty Secret name.",
			}
		}
		rendered.StaticCredentialsName = authProfile.Spec.StaticCredentials.SecretName
	}
	return rendered, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered backup auth inputs have been resolved.",
	}
}

func renderedBackupTransfer(
	transferProfile *openbaov1alpha1.OpenBaoTransferProfile,
) (*RenderedBackupTransfer, ValidationResult) {
	if transferProfile == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered backup contract does not require an explicit transfer profile.",
		}
	}

	partSize := transferProfile.Spec.PartSize
	if partSize == 0 {
		partSize = 10485760
	}
	concurrency := transferProfile.Spec.Concurrency
	if concurrency == 0 {
		concurrency = 3
	}

	return &RenderedBackupTransfer{
			PartSize:    partSize,
			Concurrency: concurrency,
		}, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered backup transfer inputs have been resolved.",
		}
}

func cloneRetention(retention *openbaov1alpha1.BackupRetention) *openbaov1alpha1.BackupRetention {
	if retention == nil {
		return nil
	}
	copy := *retention
	return &copy
}

func cloneEgressRules(rules []networkingv1.NetworkPolicyEgressRule) []networkingv1.NetworkPolicyEgressRule {
	if len(rules) == 0 {
		return nil
	}

	out := make([]networkingv1.NetworkPolicyEgressRule, len(rules))
	for i := range rules {
		rules[i].DeepCopyInto(&out[i])
	}
	return out
}

func cloneWorkloadIdentityConfig(cfg *openbaov1alpha1.WorkloadIdentityConfig) *openbaov1alpha1.WorkloadIdentityConfig {
	if cfg == nil {
		return nil
	}
	copy := &openbaov1alpha1.WorkloadIdentityConfig{}
	if len(cfg.ServiceAccountAnnotations) > 0 {
		copy.ServiceAccountAnnotations = cloneStringMap(cfg.ServiceAccountAnnotations)
	}
	if len(cfg.PodLabels) > 0 {
		copy.PodLabels = cloneStringMap(cfg.PodLabels)
	}
	return copy
}

func boundReference(name, uid string) *RenderedBoundReference {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return &RenderedBoundReference{Name: name, UID: uid}
}

func boundReferenceFromAuthProfile(profile *openbaov1alpha1.OpenBaoBackupAuthProfile) *RenderedBoundReference {
	if profile == nil {
		return nil
	}
	return boundReference(profile.Name, string(profile.UID))
}

func boundReferenceFromTransferProfile(profile *openbaov1alpha1.OpenBaoTransferProfile) *RenderedBoundReference {
	if profile == nil {
		return nil
	}
	return boundReference(profile.Name, string(profile.UID))
}
