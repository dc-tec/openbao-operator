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
	"strings"

	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

func renderedBackupSchedule(rendered *RenderedExecutionContract) (*openbaov1alpha1.BackupSchedule, ValidationResult) {
	if rendered == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract is required to build same-cluster backup configuration.",
		}
	}
	if strings.TrimSpace(rendered.Backup.Schedule) == "" && rendered.Backup.TargetRef == nil {
		return nil, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered execution contract does not require backup projection.",
		}
	}
	if rendered.Bootstrap.OperatorLifecycleAuth.Mode != openbaov1alpha1.OpenBaoBootstrapLifecycleAuthModeJWT {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster backup projection currently requires JWT-based lifecycle bootstrap auth.",
		}
	}
	if strings.TrimSpace(rendered.Backup.Schedule) == "" || rendered.Backup.TargetRef == nil {
		return nil, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered backup contract must include both a schedule and a concrete target to project into OpenBaoCluster.",
		}
	}

	target, targetResult := renderedBackupTarget(rendered)
	if !targetResult.Valid {
		return nil, targetResult
	}

	backup := &openbaov1alpha1.BackupSchedule{
		Schedule:    rendered.Backup.Schedule,
		Target:      target,
		JWTAuthRole: portauth.RoleNameBackup,
		Retention:   cloneRetention(rendered.Backup.Retention),
	}
	if rendered.Runtime.HelperImages != nil {
		backup.Image = strings.TrimSpace(rendered.Runtime.HelperImages.Backup)
	}
	return backup, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered backup contract is compatible with OpenBaoCluster backup configuration.",
	}
}

func renderedBackupTarget(rendered *RenderedExecutionContract) (openbaov1alpha1.BackupTarget, ValidationResult) {
	if rendered == nil || rendered.Backup.Backend == nil {
		return openbaov1alpha1.BackupTarget{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered backup backend is required to project same-cluster backup configuration.",
		}
	}
	if strings.TrimSpace(rendered.Backup.Location) == "" {
		return openbaov1alpha1.BackupTarget{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered backup location is required to project same-cluster backup configuration.",
		}
	}

	target := openbaov1alpha1.BackupTarget{
		Provider:           string(rendered.Backup.Backend.Provider),
		Endpoint:           rendered.Backup.Backend.Endpoint,
		Bucket:             rendered.Backup.Location,
		PathPrefix:         rendered.Backup.KeyPrefix,
		PartSize:           10485760,
		Concurrency:        3,
		InsecureSkipVerify: rendered.Backup.Backend.InsecureSkipVerify,
	}
	if rendered.Backup.Transfer != nil {
		target.PartSize = rendered.Backup.Transfer.PartSize
		target.Concurrency = rendered.Backup.Transfer.Concurrency
	}

	switch rendered.Backup.Backend.Provider {
	case openbaov1alpha1.OpenBaoObjectStorageProviderS3:
		target.Region = rendered.Backup.Backend.Region
		target.UsePathStyle = rendered.Backup.Backend.UsePathStyle
	case openbaov1alpha1.OpenBaoObjectStorageProviderGCS:
		target.GCS = &openbaov1alpha1.GCSTargetConfig{Project: rendered.Backup.Backend.GCSProject}
	case openbaov1alpha1.OpenBaoObjectStorageProviderAzure:
		if strings.TrimSpace(rendered.Backup.Backend.AzureContainer) != "" {
			return openbaov1alpha1.BackupTarget{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Same-cluster backup projection does not support Azure container overrides that diverge from the rendered backup location.",
			}
		}
		target.Azure = &openbaov1alpha1.AzureTargetConfig{
			StorageAccount: rendered.Backup.Backend.AzureStorageAccount,
		}
	default:
		return openbaov1alpha1.BackupTarget{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Same-cluster backup projection supports only object-storage provider shapes recognized by OpenBaoCluster.",
		}
	}

	if rendered.Backup.Auth == nil {
		return target, ValidationResult{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "Rendered same-cluster backup target uses default storage identity posture.",
		}
	}

	switch rendered.Backup.Auth.Mode {
	case openbaov1alpha1.OpenBaoBackupAuthModeStaticCredentials:
		if strings.TrimSpace(rendered.Backup.Auth.StaticCredentialsName) == "" {
			return openbaov1alpha1.BackupTarget{}, ValidationResult{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "Rendered static backup credentials must include a Secret name.",
			}
		}
		target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: rendered.Backup.Auth.StaticCredentialsName}
	case openbaov1alpha1.OpenBaoBackupAuthModeWorkloadIdentity:
		target.WorkloadIdentity = cloneWorkloadIdentityConfig(rendered.Backup.Auth.WorkloadIdentity)
		if strings.TrimSpace(rendered.Backup.Auth.RoleARN) != "" {
			if rendered.Backup.Backend.Provider != openbaov1alpha1.OpenBaoObjectStorageProviderS3 {
				return openbaov1alpha1.BackupTarget{}, ValidationResult{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonInvalid,
					Message: "Same-cluster backup projection supports RoleARN only for S3-compatible backup targets.",
				}
			}
			target.RoleARN = rendered.Backup.Auth.RoleARN
		}
	default:
		return openbaov1alpha1.BackupTarget{}, ValidationResult{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Rendered backup auth mode is not compatible with OpenBaoCluster backup configuration.",
		}
	}

	return target, ValidationResult{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Rendered same-cluster backup target is compatible with OpenBaoCluster backup configuration.",
	}
}
