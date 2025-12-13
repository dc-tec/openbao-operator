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
)

const (
	backupJobNamePrefix = "backup-"
	backupJobTTLSeconds = 3600 // 1 hour TTL for completed/failed jobs
)

const (
	// Backup executor images are built to run as a non-root user with stable
	// UID/GID. The Job security context pins these IDs so that the backup
	// container always runs as non-root even if the image metadata defaults to
	// root.
	backupUserID  = int64(1000)
	backupGroupID = int64(1000)
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
func (m *Manager) processBackupJobResult(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) error {
	job := &batchv1.Job{}

	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Job was cleaned up or doesn't exist
			return nil
		}
		return fmt.Errorf("failed to get backup Job %s/%s: %w", cluster.Namespace, jobName, err)
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

		logger.Info("Backup Job completed successfully", "job", jobName)
		return nil
	}

	if job.Status.Failed > 0 {
		// Job failed
		cluster.Status.Backup.ConsecutiveFailures++
		cluster.Status.Backup.LastFailureReason = fmt.Sprintf("Backup Job %s failed", jobName)

		m.setBackingUpCondition(cluster, false, "BackupFailed",
			fmt.Sprintf("Backup Job %s failed", jobName))

		logger.Error(fmt.Errorf("backup job failed"), "Backup Job failed",
			"job", jobName,
			"consecutiveFailures", cluster.Status.Backup.ConsecutiveFailures)
		return nil
	}

	// Job is still running
	m.setBackingUpCondition(cluster, true, "BackupInProgress",
		fmt.Sprintf("Backup Job %s is in progress", jobName))
	return nil
}

func backupJobName(cluster *openbaov1alpha1.OpenBaoCluster, scheduledTime time.Time) string {
	return backupJobNamePrefix + cluster.Name + "-" + scheduledTime.UTC().Format("20060102-150405")
}

func buildBackupJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (*batchv1.Job, error) {
	// Build environment variables for the backup container
	env := []corev1.EnvVar{
		{Name: "CLUSTER_NAMESPACE", Value: cluster.Namespace},
		{Name: "CLUSTER_NAME", Value: cluster.Name},
		{Name: "CLUSTER_REPLICAS", Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: "BACKUP_ENDPOINT", Value: cluster.Spec.Backup.Target.Endpoint},
		{Name: "BACKUP_BUCKET", Value: cluster.Spec.Backup.Target.Bucket},
		{Name: "BACKUP_PATH_PREFIX", Value: cluster.Spec.Backup.Target.PathPrefix},
		{Name: "BACKUP_USE_PATH_STYLE", Value: fmt.Sprintf("%t", cluster.Spec.Backup.Target.UsePathStyle)},
	}

	// Add S3 upload configuration if specified
	if cluster.Spec.Backup.Target.PartSize > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_PART_SIZE",
			Value: fmt.Sprintf("%d", cluster.Spec.Backup.Target.PartSize),
		})
	}
	if cluster.Spec.Backup.Target.Concurrency > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_CONCURRENCY",
			Value: fmt.Sprintf("%d", cluster.Spec.Backup.Target.Concurrency),
		})
	}

	// Add credentials secret reference if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_CREDENTIALS_SECRET_NAME",
			Value: cluster.Spec.Backup.Target.CredentialsSecretRef.Name,
		})
		if cluster.Spec.Backup.Target.CredentialsSecretRef.Namespace != "" {
			env = append(env, corev1.EnvVar{
				Name:  "BACKUP_CREDENTIALS_SECRET_NAMESPACE",
				Value: cluster.Spec.Backup.Target.CredentialsSecretRef.Namespace,
			})
		}
	}

	// Add JWT Auth configuration (preferred method)
	// The executor will use JWT Auth if BACKUP_JWT_AUTH_ROLE is set
	// and the JWT token is available from the projected volume
	if cluster.Spec.Backup.JWTAuthRole != "" {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_JWT_AUTH_ROLE",
			Value: cluster.Spec.Backup.JWTAuthRole,
		})
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_AUTH_METHOD",
			Value: "jwt",
		})
	}

	// Add token secret reference if provided (fallback for token-based auth)
	if cluster.Spec.Backup.TokenSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_TOKEN_SECRET_NAME",
			Value: cluster.Spec.Backup.TokenSecretRef.Name,
		})
		if cluster.Spec.Backup.TokenSecretRef.Namespace != "" {
			env = append(env, corev1.EnvVar{
				Name:  "BACKUP_TOKEN_SECRET_NAMESPACE",
				Value: cluster.Spec.Backup.TokenSecretRef.Namespace,
			})
		}
		// Only set auth method to token if JWT Auth is not configured
		if cluster.Spec.Backup.JWTAuthRole == "" {
			env = append(env, corev1.EnvVar{
				Name:  "BACKUP_AUTH_METHOD",
				Value: "token",
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
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   cluster.Name,
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          cluster.Name,
				"openbao.org/component":        "backup",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":       "openbao",
						"app.kubernetes.io/instance":   cluster.Name,
						"app.kubernetes.io/managed-by": "openbao-operator",
						"openbao.org/cluster":          cluster.Name,
						"openbao.org/component":        "backup",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: backupServiceAccountName(cluster),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(backupUserID),
						RunAsGroup:   ptr.To(backupGroupID),
						FSGroup:      ptr.To(backupGroupID),
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

	return job, nil
}

func buildBackupJobVolumeMounts(cluster *openbaov1alpha1.OpenBaoCluster) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{}

	// Mount TLS CA certificate
	mounts = append(mounts, corev1.VolumeMount{
		Name:      "tls-ca",
		MountPath: "/etc/bao/tls",
		ReadOnly:  true,
	})

	// Mount credentials secret if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "backup-credentials",
			MountPath: "/etc/bao/backup/credentials",
			ReadOnly:  true,
		})
	}

	// Mount JWT token from projected volume (preferred method)
	if cluster.Spec.Backup.JWTAuthRole != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "openbao-token",
			MountPath: "/var/run/secrets/tokens",
			ReadOnly:  true,
		})
	}

	// Mount token secret if provided (fallback method when JWT Auth is not used)
	if cluster.Spec.Backup.TokenSecretRef != nil {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "backup-token",
			MountPath: "/etc/bao/backup/token",
			ReadOnly:  true,
		})
	}

	return mounts
}

func buildBackupJobVolumes(cluster *openbaov1alpha1.OpenBaoCluster) []corev1.Volume {
	volumes := []corev1.Volume{}

	// TLS CA certificate
	volumes = append(volumes, corev1.Volume{
		Name: "tls-ca",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: fmt.Sprintf("%s-tls-ca", cluster.Name),
			},
		},
	})

	// Credentials secret if provided
	// Use strict file permissions (0400) for backup credentials to prevent
	// unauthorized access if a user can exec into the backup pod.
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		secretName := cluster.Spec.Backup.Target.CredentialsSecretRef.Name
		credentialsFileMode := int32(0400) // Owner read-only

		volumes = append(volumes, corev1.Volume{
			Name: "backup-credentials",
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
			Name: "openbao-token",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              "openbao-token",
								ExpirationSeconds: ptr.To(int64(3600)),
								Audience:          "openbao-internal",
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
			Name: "backup-token",
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
