package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
)

const (
	backupJobNamePrefix = "backup-"
	backupJobTTLSeconds = 3600 // 1 hour TTL for completed/failed jobs
)

const (
	awsWebIdentityVolumeName     = "aws-iam-token"
	awsWebIdentityMountPath      = "/var/run/secrets/aws"
	awsWebIdentityTokenFile      = "/var/run/secrets/aws/token" // #nosec G101 -- This is a mount path constant, not a credential
	awsWebIdentityTokenAudience  = "sts.amazonaws.com"          // #nosec G101 -- This is a constant, not a credential
	openBaoTokenVolumeName       = "openbao-token"
	openBaoTokenMountPath        = "/var/run/secrets/tokens" // #nosec G101 -- This is a mount path constant, not a credential
	openBaoTokenFileRelativePath = "openbao-token"
	openBaoTokenAudience         = "openbao-internal"
	backupCredentialsVolumeName  = "backup-credentials"          // #nosec G101 -- This is a volume name constant, not a credential
	backupCredentialsMountPath   = "/etc/bao/backup/credentials" // #nosec G101 -- This is a mount path constant, not a credential
	backupTokenVolumeName        = "backup-token"
	backupTokenMountPath         = "/etc/bao/backup/token" // #nosec G101 -- This is a mount path constant, not a credential
	backupTLSCAVolumeName        = "tls-ca"
)

// ensureBackupJob creates or updates a Kubernetes Job for executing the backup.
// Returns true if a Job was created or is already running, false if backup should not proceed.
func (m *Manager) ensureBackupJob(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (bool, error) {
	job := &batchv1.Job{}

	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get backup Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		// Job doesn't exist - create it
		logger.Info("Creating backup Job", "job", jobName)
		job, buildErr := buildBackupJob(cluster, jobName)
		if buildErr != nil {
			return false, fmt.Errorf("failed to build backup Job %s/%s: %w", cluster.Namespace, jobName, buildErr)
		}

		// Set OwnerReference for garbage collection
		if err := controllerutil.SetControllerReference(cluster, job, m.scheme); err != nil {
			return false, fmt.Errorf("failed to set owner reference on backup Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		if err := m.client.Create(ctx, job); err != nil {
			return false, fmt.Errorf("failed to create backup Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		logger.Info("Backup Job created", "job", jobName)
		return true, nil
	}

	// Job exists - check its status
	if job.Status.Succeeded > 0 {
		logger.Info("Backup Job completed successfully", "job", jobName)
		return false, nil // Job completed, backup manager will process result
	}

	if job.Status.Failed > 0 {
		// Check if we should retry (e.g., if it's a transient error)
		// For now, we'll let the Job be cleaned up and a new one created on next reconcile
		logger.Info("Backup Job failed", "job", jobName, "failed", job.Status.Failed)
		return false, nil
	}

	// Job is still running or pending
	logger.V(1).Info("Backup Job is in progress", "job", jobName,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed)
	return true, nil
}

// processBackupJobResult processes the result of a completed backup Job and updates cluster status.
// Returns (statusUpdated, error) where statusUpdated indicates if the status was modified
// (job completed successfully or failed). This is used to determine if a requeue is needed
// to persist the status update.
func (m *Manager) processBackupJobResult(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (bool, error) {
	job := &batchv1.Job{}

	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Job was cleaned up or doesn't exist
			return false, nil
		}
		return false, fmt.Errorf("failed to get backup Job %s/%s: %w", cluster.Namespace, jobName, err)
	}

	// Check Job status
	if job.Status.Succeeded > 0 {
		// Job succeeded - extract result from Job annotations or logs
		// For now, we'll update status based on Job completion
		// In a full implementation, the Job would write results to a ConfigMap or annotation
		now := metav1.Now()
		cluster.Status.Backup.LastBackupTime = &now
		cluster.Status.Backup.ConsecutiveFailures = 0
		cluster.Status.Backup.LastFailureReason = ""

		m.setBackingUpCondition(cluster, false, "BackupSucceeded",
			fmt.Sprintf("Backup Job %s completed successfully", jobName))

		logger.Info("Backup Job completed successfully, status updated", "job", jobName, "lastBackupTime", now)
		return true, nil // Status was updated - request requeue to persist
	}

	if job.Status.Failed > 0 {
		// Job failed
		cluster.Status.Backup.ConsecutiveFailures++
		cluster.Status.Backup.LastFailureReason = fmt.Sprintf("Backup Job %s failed", jobName)

		m.setBackingUpCondition(cluster, false, "BackupFailed",
			fmt.Sprintf("Backup Job %s failed", jobName))

		logger.Error(fmt.Errorf("backup job failed"), "Backup Job failed, status updated",
			"job", jobName,
			"consecutiveFailures", cluster.Status.Backup.ConsecutiveFailures)
		return true, nil // Status was updated - request requeue to persist
	}

	// Job is still running
	m.setBackingUpCondition(cluster, true, "BackupInProgress",
		fmt.Sprintf("Backup Job %s is in progress", jobName))
	return false, nil // Status updated but job still running - no requeue needed yet
}

func backupJobName(cluster *openbaov1alpha1.OpenBaoCluster, scheduledTime time.Time) string {
	return backupJobNamePrefix + cluster.Name + "-" + scheduledTime.UTC().Format("20060102-150405")
}

func buildBackupJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (*batchv1.Job, error) {
	region := cluster.Spec.Backup.Target.Region
	if region == "" {
		region = constants.DefaultS3Region
	}

	// Build environment variables for the backup container
	env := []corev1.EnvVar{
		{Name: constants.EnvClusterNamespace, Value: cluster.Namespace},
		{Name: constants.EnvClusterName, Value: cluster.Name},
		{Name: constants.EnvClusterReplicas, Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: constants.EnvBackupEndpoint, Value: cluster.Spec.Backup.Target.Endpoint},
		{Name: constants.EnvBackupBucket, Value: cluster.Spec.Backup.Target.Bucket},
		{Name: constants.EnvBackupPathPrefix, Value: cluster.Spec.Backup.Target.PathPrefix},
		{Name: constants.EnvBackupRegion, Value: region},
		{Name: constants.EnvBackupUsePathStyle, Value: fmt.Sprintf("%t", cluster.Spec.Backup.Target.UsePathStyle)},
	}

	if cluster.Spec.Backup.Target.RoleARN != "" {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvAWSRoleARN,
			Value: cluster.Spec.Backup.Target.RoleARN,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvAWSWebIdentityTokenFile,
			Value: awsWebIdentityTokenFile,
		})
	}

	// Add S3 upload configuration if specified
	if cluster.Spec.Backup.Target.PartSize > 0 {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupPartSize,
			Value: fmt.Sprintf("%d", cluster.Spec.Backup.Target.PartSize),
		})
	}
	if cluster.Spec.Backup.Target.Concurrency > 0 {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupConcurrency,
			Value: fmt.Sprintf("%d", cluster.Spec.Backup.Target.Concurrency),
		})
	}

	// Add credentials secret reference if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupCredentialsSecretName,
			Value: cluster.Spec.Backup.Target.CredentialsSecretRef.Name,
		})
		if cluster.Spec.Backup.Target.CredentialsSecretRef.Namespace != "" {
			env = append(env, corev1.EnvVar{
				Name:  constants.EnvBackupCredentialsSecretNamespace,
				Value: cluster.Spec.Backup.Target.CredentialsSecretRef.Namespace,
			})
		}
	}

	// Add JWT Auth configuration (preferred method)
	// The executor will use JWT Auth if BACKUP_JWT_AUTH_ROLE is set
	// and the JWT token is available from the projected volume
	if cluster.Spec.Backup.JWTAuthRole != "" {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupJWTAuthRole,
			Value: cluster.Spec.Backup.JWTAuthRole,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodJWT,
		})
	}

	// Add token secret reference if provided (fallback for token-based auth)
	if cluster.Spec.Backup.TokenSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupTokenSecretName,
			Value: cluster.Spec.Backup.TokenSecretRef.Name,
		})
		if cluster.Spec.Backup.TokenSecretRef.Namespace != "" {
			env = append(env, corev1.EnvVar{
				Name:  constants.EnvBackupTokenSecretNamespace,
				Value: cluster.Spec.Backup.TokenSecretRef.Namespace,
			})
		}
		// Only set auth method to token if JWT Auth is not configured
		if cluster.Spec.Backup.JWTAuthRole == "" {
			env = append(env, corev1.EnvVar{
				Name:  constants.EnvBackupAuthMethod,
				Value: constants.BackupAuthMethodToken,
			})
		}
	}

	backoffLimit := int32(0) // Don't retry failed backups automatically
	ttlSecondsAfterFinished := int32(backupJobTTLSeconds)

	image := getBackupExecutorImage(cluster)
	if image == "" {
		return nil, fmt.Errorf("backup executor image is required (spec.backup.executorImage)")
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: "backup",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
						constants.LabelAppInstance:      cluster.Name,
						constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
						constants.LabelOpenBaoCluster:   cluster.Name,
						constants.LabelOpenBaoComponent: "backup",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: backupServiceAccountName(cluster),
					// Explicitly disable default token mounts. The backup Job only mounts a
					// projected ServiceAccount token when JWT auth and/or S3 Web Identity is
					// configured.
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(constants.UserBackup),
						RunAsGroup:   ptr.To(constants.GroupBackup),
						FSGroup:      ptr.To(constants.GroupBackup),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "backup",
							Image: image,
							SecurityContext: &corev1.SecurityContext{
								// Prevent privilege escalation (sudo, setuid binaries)
								AllowPrivilegeEscalation: ptr.To(false),
								// Drop ALL capabilities.
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								// Read-only root filesystem to prevent runtime modification
								ReadOnlyRootFilesystem: ptr.To(true),
								// Run as non-root (inherited from PodSecurityContext, but explicit here is safe)
								RunAsNonRoot: ptr.To(true),
							},
							Env: env,
							// Mount secrets for credentials and tokens
							VolumeMounts: buildBackupJobVolumeMounts(cluster),
						},
					},
					Volumes: buildBackupJobVolumes(cluster),
				},
			},
		},
	}

	if cluster.Spec.WorkloadHardening != nil && cluster.Spec.WorkloadHardening.AppArmorEnabled {
		job.Spec.Template.Spec.SecurityContext.AppArmorProfile = &corev1.AppArmorProfile{
			Type: corev1.AppArmorProfileTypeRuntimeDefault,
		}
	}

	return job, nil
}

func buildBackupJobVolumeMounts(cluster *openbaov1alpha1.OpenBaoCluster) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}

	// Mount TLS CA certificate
	mounts = append(mounts, corev1.VolumeMount{
		Name:      backupTLSCAVolumeName,
		MountPath: constants.PathTLS,
		ReadOnly:  true,
	})

	if cluster.Spec.Backup.Target.RoleARN != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      awsWebIdentityVolumeName,
			MountPath: awsWebIdentityMountPath,
			ReadOnly:  true,
		})
	}

	// Mount credentials secret if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      backupCredentialsVolumeName,
			MountPath: backupCredentialsMountPath,
			ReadOnly:  true,
		})
	}

	// Mount JWT token from projected volume (preferred method)
	if cluster.Spec.Backup.JWTAuthRole != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      openBaoTokenVolumeName,
			MountPath: openBaoTokenMountPath,
			ReadOnly:  true,
		})
	}

	// Mount token secret if provided (fallback method when JWT Auth is not used)
	if cluster.Spec.Backup.TokenSecretRef != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      backupTokenVolumeName,
			MountPath: backupTokenMountPath,
			ReadOnly:  true,
		})
	}

	return mounts
}

func buildBackupJobVolumes(cluster *openbaov1alpha1.OpenBaoCluster) []corev1.Volume {
	volumes := []corev1.Volume{}

	// TLS CA certificate
	volumes = append(volumes, corev1.Volume{
		Name: backupTLSCAVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: cluster.Name + constants.SuffixTLSCA,
			},
		},
	})

	if cluster.Spec.Backup.Target.RoleARN != "" {
		volumes = append(volumes, corev1.Volume{
			Name: awsWebIdentityVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              "token",
								ExpirationSeconds: ptr.To(int64(3600)),
								Audience:          awsWebIdentityTokenAudience,
							},
						},
					},
				},
			},
		})
	}

	// Credentials secret if provided
	// Use strict file permissions (0400) for backup credentials to prevent
	// unauthorized access if a user can exec into the backup pod.
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		secretName := cluster.Spec.Backup.Target.CredentialsSecretRef.Name
		credentialsFileMode := int32(0400) // Owner read-only

		volumes = append(volumes, corev1.Volume{
			Name: backupCredentialsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &credentialsFileMode,
				},
			},
		})
	}

	// JWT token from projected volume (preferred method)
	if cluster.Spec.Backup.JWTAuthRole != "" {
		volumes = append(volumes, corev1.Volume{
			Name: openBaoTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              openBaoTokenFileRelativePath,
								ExpirationSeconds: ptr.To(int64(3600)),
								Audience:          openBaoTokenAudience,
							},
						},
					},
				},
			},
		})
	}

	// Token secret (fallback method when JWT Auth is not used)
	// Note: Secret must be in the same namespace as the Job Pod
	if cluster.Spec.Backup.TokenSecretRef != nil {
		secretName := cluster.Spec.Backup.TokenSecretRef.Name

		tokenFileMode := int32(0400) // Owner read-only for security
		volumes = append(volumes, corev1.Volume{
			Name: backupTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &tokenFileMode,
				},
			},
		})
	}

	return volumes
}

// getBackupExecutorImage returns the backup executor image to use.
// Defaults to "openbao/backup-executor:v0.1.0" if not specified in the cluster spec.
func getBackupExecutorImage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.Backup == nil || strings.TrimSpace(cluster.Spec.Backup.ExecutorImage) == "" {
		return ""
	}
	return cluster.Spec.Backup.ExecutorImage
}
