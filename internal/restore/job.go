package restore

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/auth"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

const (
	// Volume and mount names for restore jobs
	restoreTLSCAVolumeName        = "tls-ca"
	restoreTLSCAMountPath         = constants.PathTLS
	restoreS3CredentialsVolume    = "s3-credentials"
	restoreS3CredentialsMountPath = constants.PathBackupCredentials // Same path as backup for LoadExecutorConfig
	restoreJWTTokenVolumeName     = "jwt-token"
	restoreJWTTokenMountPath      = "/var/run/secrets/tokens" // #nosec G101 -- mount path not credential
	restoreTokenVolumeName        = "restore-token"
	restoreTokenMountPath         = "/etc/bao/restore/token" // #nosec G101 -- mount path not credential
)

func getRestoreExecutorImage(restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if restore.Spec.ExecutorImage != "" {
		return restore.Spec.ExecutorImage, nil
	}
	if cluster.Spec.Backup != nil && cluster.Spec.Backup.ExecutorImage != "" {
		return cluster.Spec.Backup.ExecutorImage, nil
	}
	return "", fmt.Errorf("no executor image specified in restore or cluster backup config")
}

// buildRestoreJob creates a Kubernetes Job for executing the restore.
func (m *Manager) buildRestoreJob(restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster, verifiedExecutorDigest string) (*batchv1.Job, error) {
	jobName := restoreJobName(restore)
	labels := restoreLabels(cluster)

	executorImage, err := getRestoreExecutorImage(restore, cluster)
	if err != nil {
		return nil, err
	}

	image := verifiedExecutorDigest
	if image == "" {
		image = executorImage
	}

	// Build environment variables
	envVars := buildRestoreEnvVars(restore, cluster)

	// Build volumes and mounts
	volumes := buildRestoreVolumes(restore, cluster)
	volumeMounts := buildRestoreVolumeMounts(restore, cluster)

	// Build container
	container := corev1.Container{
		Name:  "restore",
		Image: image,
		Env:   envVars,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(true),
			RunAsNonRoot:             ptr.To(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		// SECURITY: Resource limits prevent restore jobs from exhausting node resources
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		VolumeMounts: volumeMounts,
	}

	// Job backoff limit - allow a few retries for transient failures
	backoffLimit := int32(3)
	ttlSeconds := int32(RestoreJobTTLSeconds)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: restore.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           restoreServiceAccountName(cluster),
					AutomountServiceAccountToken: ptr.To(false),
					RestartPolicy:                corev1.RestartPolicyOnFailure,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(constants.UserBackup),
						RunAsGroup:   ptr.To(constants.GroupBackup),
						FSGroup:      ptr.To(constants.GroupBackup),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{container},
					Volumes:    volumes,
				},
			},
		},
	}

	return job, nil
}

// buildRestoreEnvVars builds environment variables for the restore job.
func buildRestoreEnvVars(restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		// Set executor mode to restore
		{
			Name:  "EXECUTOR_MODE",
			Value: "restore",
		},
		// Cluster info
		{
			Name:  constants.EnvClusterName,
			Value: cluster.Name,
		},
		{
			Name:  constants.EnvClusterNamespace,
			Value: cluster.Namespace,
		},
		{
			Name:  constants.EnvClusterReplicas,
			Value: fmt.Sprintf("%d", cluster.Spec.Replicas),
		},
		// BACKUP_* env vars are required by LoadExecutorConfig for validation.
		// The executor will use RESTORE_* values when available.
		{
			Name:  constants.EnvBackupEndpoint,
			Value: restore.Spec.Source.Target.Endpoint,
		},
		{
			Name:  constants.EnvBackupBucket,
			Value: restore.Spec.Source.Target.Bucket,
		},
		{
			Name:  constants.EnvBackupRegion,
			Value: restore.Spec.Source.Target.Region,
		},
		{
			Name:  constants.EnvBackupUsePathStyle,
			Value: fmt.Sprintf("%t", restore.Spec.Source.Target.UsePathStyle),
		},
		// Restore-specific overrides (used by runRestore function)
		{
			Name:  "RESTORE_KEY",
			Value: restore.Spec.Source.Key,
		},
		{
			Name:  "RESTORE_BUCKET",
			Value: restore.Spec.Source.Target.Bucket,
		},
		{
			Name:  "RESTORE_ENDPOINT",
			Value: restore.Spec.Source.Target.Endpoint,
		},
		{
			Name:  "RESTORE_REGION",
			Value: restore.Spec.Source.Target.Region,
		},
		{
			Name:  "RESTORE_USE_PATH_STYLE",
			Value: fmt.Sprintf("%t", restore.Spec.Source.Target.UsePathStyle),
		},
	}

	// JWT auth configuration
	if restore.Spec.JWTAuthRole != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupJWTAuthRole,
			Value: restore.Spec.JWTAuthRole,
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodJWT,
		})
	} else if restore.Spec.TokenSecretRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodToken,
		})
	}

	// S3 credentials if using secret ref
	if restore.Spec.Source.Target.CredentialsSecretRef != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: "AWS_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: restore.Spec.Source.Target.CredentialsSecretRef.Name,
					},
					Key: "accessKeyId",
				},
			},
		})
		envVars = append(envVars, corev1.EnvVar{
			Name: "AWS_SECRET_ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: restore.Spec.Source.Target.CredentialsSecretRef.Name,
					},
					Key: "secretAccessKey",
				},
			},
		})
	}

	return envVars
}

// buildRestoreVolumes builds volumes for the restore job.
func buildRestoreVolumes(restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) []corev1.Volume {
	var volumes []corev1.Volume

	// TLS CA volume (if TLS is enabled)
	if cluster.Spec.TLS.Enabled {
		tlsSecretName := cluster.Name + constants.SuffixTLSCA
		volumes = append(volumes, corev1.Volume{
			Name: restoreTLSCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: tlsSecretName,
				},
			},
		})
	}

	// JWT token volume (if using JWT auth)
	if restore.Spec.JWTAuthRole != "" {
		audience := auth.OpenBaoJWTAudience()
		expirationSeconds := int64(3600)
		volumes = append(volumes, corev1.Volume{
			Name: restoreJWTTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Audience:          audience,
								ExpirationSeconds: &expirationSeconds,
								Path:              "openbao-token",
							},
						},
					},
				},
			},
		})
	}

	// Static token volume (if using token auth)
	if restore.Spec.TokenSecretRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: restoreTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: restore.Spec.TokenSecretRef.Name,
				},
			},
		})
	}

	// S3 credentials volume (if using credentials secret)
	if restore.Spec.Source.Target.CredentialsSecretRef != nil {
		defaultMode := int32(0400)
		volumes = append(volumes, corev1.Volume{
			Name: restoreS3CredentialsVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  restore.Spec.Source.Target.CredentialsSecretRef.Name,
					DefaultMode: &defaultMode,
				},
			},
		})
	}

	return volumes
}

// buildRestoreVolumeMounts builds volume mounts for the restore container.
func buildRestoreVolumeMounts(restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) []corev1.VolumeMount {
	var mounts []corev1.VolumeMount

	// TLS CA mount
	if cluster.Spec.TLS.Enabled {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      restoreTLSCAVolumeName,
			MountPath: restoreTLSCAMountPath,
			ReadOnly:  true,
		})
	}

	// JWT token mount
	if restore.Spec.JWTAuthRole != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      restoreJWTTokenVolumeName,
			MountPath: restoreJWTTokenMountPath,
			ReadOnly:  true,
		})
	}

	// Static token mount
	if restore.Spec.TokenSecretRef != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      restoreTokenVolumeName,
			MountPath: restoreTokenMountPath,
			ReadOnly:  true,
		})
	}

	// S3 credentials mount
	if restore.Spec.Source.Target.CredentialsSecretRef != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      restoreS3CredentialsVolume,
			MountPath: restoreS3CredentialsMountPath,
			ReadOnly:  true,
		})
	}

	return mounts
}

// boolPtr returns a pointer to the given bool value.
