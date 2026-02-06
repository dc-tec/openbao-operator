package storageenv

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// EffectiveProvider normalizes an empty provider to the default.
func EffectiveProvider(provider string) string {
	if provider == "" {
		return constants.StorageProviderS3
	}
	return provider
}

// EffectiveJWTRole returns the configured role or a default when OIDC is enabled.
func EffectiveJWTRole(configuredRole string, oidcEnabled bool, defaultRole string) string {
	if configuredRole != "" {
		return configuredRole
	}
	if oidcEnabled {
		return defaultRole
	}
	return ""
}

// AppendProviderEnvVars appends provider-specific BACKUP_* env vars.
func AppendProviderEnvVars(envVars []corev1.EnvVar, target openbaov1alpha1.BackupTarget) []corev1.EnvVar {
	if target.InsecureSkipVerify {
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupInsecureSkipVerify,
			Value: "true",
		})
	}

	switch EffectiveProvider(target.Provider) {
	case constants.StorageProviderS3:
		region := target.Region
		if region == "" {
			region = constants.DefaultS3Region
		}
		envVars = append(envVars, corev1.EnvVar{Name: constants.EnvBackupRegion, Value: region})
		envVars = append(envVars, corev1.EnvVar{Name: constants.EnvBackupUsePathStyle, Value: fmt.Sprintf("%t", target.UsePathStyle)})
	case constants.StorageProviderGCS:
		if target.GCS != nil && target.GCS.Project != "" {
			envVars = append(envVars, corev1.EnvVar{Name: constants.EnvBackupGCSProject, Value: target.GCS.Project})
		}
	case constants.StorageProviderAzure:
		if target.Azure != nil {
			if target.Azure.StorageAccount != "" {
				envVars = append(envVars, corev1.EnvVar{Name: constants.EnvBackupAzureStorageAccount, Value: target.Azure.StorageAccount})
			}
			if target.Azure.Container != "" {
				envVars = append(envVars, corev1.EnvVar{Name: constants.EnvBackupAzureContainer, Value: target.Azure.Container})
			}
		}
	}

	return envVars
}

// AppendRestoreProviderEnvVars appends restore-only provider env vars.
func AppendRestoreProviderEnvVars(envVars []corev1.EnvVar, target openbaov1alpha1.BackupTarget) []corev1.EnvVar {
	if EffectiveProvider(target.Provider) != constants.StorageProviderS3 {
		return envVars
	}

	region := target.Region
	if region == "" {
		region = constants.DefaultS3Region
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  constants.EnvRestoreRegion,
		Value: region,
	})
	envVars = append(envVars, corev1.EnvVar{
		Name:  constants.EnvRestoreUsePathStyle,
		Value: fmt.Sprintf("%t", target.UsePathStyle),
	})
	return envVars
}

// AppendAuthEnvVars appends JWT/token auth env vars used by backup executor logic.
func AppendAuthEnvVars(envVars []corev1.EnvVar, jwtRole string, hasTokenSecret bool) []corev1.EnvVar {
	if jwtRole != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupJWTAuthRole,
			Value: jwtRole,
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodJWT,
		})
		return envVars
	}

	if hasTokenSecret {
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodToken,
		})
	}
	return envVars
}
