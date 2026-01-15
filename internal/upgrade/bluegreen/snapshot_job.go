package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

// ensurePreUpgradeSnapshotJob creates or checks the status of the pre-upgrade snapshot Job.
func (m *Manager) ensurePreUpgradeSnapshotJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (*JobResult, error) {
	if cluster.Spec.Backup == nil || cluster.Spec.Backup.Target.Endpoint == "" {
		return nil, fmt.Errorf("backup configuration required for pre-upgrade snapshot")
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		return nil, operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so snapshot Jobs can reach the object storage endpoint",
			)),
		)
	}

	// ExecutorImage defaults to constants.DefaultBackupImage() when not specified
	if err := backup.EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster); err != nil {
		return nil, fmt.Errorf("failed to ensure backup ServiceAccount for snapshot job: %w", err)
	}
	if err := backup.EnsureBackupRBAC(ctx, m.client, m.scheme, cluster); err != nil {
		return nil, fmt.Errorf("failed to ensure backup RBAC for snapshot job: %w", err)
	}

	return ensureJob(ctx, m.client, m.scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		verifiedExecutorDigest, err := m.verifyImageDigest(
			ctx,
			logger,
			cluster,
			cluster.Spec.Backup.ExecutorImage,
			constants.ReasonBlueGreenSnapshotImageVerificationFailed,
			"Pre-upgrade snapshot executor image verification failed",
		)
		if err != nil {
			return nil, err
		}
		return m.buildSnapshotJob(cluster, jobName, "pre-upgrade", verifiedExecutorDigest), nil
	}, "component", ComponentUpgradeSnapshot, "phase", "pre-upgrade")
}

// buildSnapshotJob creates a backup Job spec for upgrade snapshots.
func (m *Manager) buildSnapshotJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName, phase string, verifiedExecutorDigest string) *batchv1.Job {
	region := cluster.Spec.Backup.Target.Region
	if region == "" {
		region = "us-east-1"
	}

	// Compute StatefulSet name for pod discovery
	// For pre-upgrade snapshots, use the Blue revision (current stable pods)
	statefulSetName := cluster.Name
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
		statefulSetName = fmt.Sprintf("%s-%s", cluster.Name, cluster.Status.BlueGreen.BlueRevision)
	}

	// Build environment variables for the backup container
	env := []corev1.EnvVar{
		{Name: constants.EnvClusterNamespace, Value: cluster.Namespace},
		{Name: constants.EnvClusterName, Value: cluster.Name},
		{Name: constants.EnvStatefulSetName, Value: statefulSetName},
		{Name: constants.EnvClusterReplicas, Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: constants.EnvBackupEndpoint, Value: cluster.Spec.Backup.Target.Endpoint},
		{Name: constants.EnvBackupBucket, Value: cluster.Spec.Backup.Target.Bucket},
		{Name: constants.EnvBackupPathPrefix, Value: cluster.Spec.Backup.Target.PathPrefix},
		{Name: constants.EnvBackupRegion, Value: region},
		{Name: constants.EnvBackupUsePathStyle, Value: fmt.Sprintf("%t", cluster.Spec.Backup.Target.UsePathStyle)},
	}

	// Smart Client Limits
	if m.clientConfig.RateLimitQPS > 0 {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvClientQPS,
			Value: fmt.Sprintf("%f", m.clientConfig.RateLimitQPS),
		})
	}
	if m.clientConfig.RateLimitBurst > 0 {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvClientBurst,
			Value: fmt.Sprintf("%d", m.clientConfig.RateLimitBurst),
		})
	}
	if m.clientConfig.CircuitBreakerFailureThreshold > 0 {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvClientCircuitBreakerFailureThreshold,
			Value: fmt.Sprintf("%d", m.clientConfig.CircuitBreakerFailureThreshold),
		})
	}
	if m.clientConfig.CircuitBreakerOpenDuration > 0 {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvClientCircuitBreakerOpenDuration,
			Value: m.clientConfig.CircuitBreakerOpenDuration.String(),
		})
	}

	// Add role ARN if configured
	if cluster.Spec.Backup.Target.RoleARN != "" {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvAWSRoleARN,
			Value: cluster.Spec.Backup.Target.RoleARN,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvAWSWebIdentityTokenFile,
			Value: "/var/run/secrets/aws/token",
		})
	}

	// Add JWT Auth configuration if available
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

	backoffLimit := int32(0) // Don't retry
	ttlSecondsAfterFinished := int32(3600)

	// Build volume mounts
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tls-ca",
			MountPath: constants.PathTLS,
			ReadOnly:  true,
		},
	}

	// Add JWT token mount if configured
	if cluster.Spec.Backup.JWTAuthRole != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "openbao-token",
			MountPath: "/var/run/secrets/tokens",
			ReadOnly:  true,
		})
	}

	// Add credentials secret mount if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "backup-credentials",
			MountPath: constants.PathBackupCredentials,
			ReadOnly:  true,
		})
	}

	// Build volumes
	volumes := []corev1.Volume{
		{
			Name: "tls-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cluster.Name + constants.SuffixTLSCA,
				},
			},
		},
	}

	// Add JWT token volume if configured
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
								Audience:          constants.TokenAudienceOpenBaoInternal,
							},
						},
					},
				},
			},
		})
	}

	// Add credentials secret volume if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		credentialsFileMode := int32(0400) // Owner read-only
		volumes = append(volumes, corev1.Volume{
			Name: "backup-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  cluster.Spec.Backup.Target.CredentialsSecretRef.Name,
					DefaultMode: &credentialsFileMode,
				},
			},
		})
	}

	// Service account name - use backup service account
	serviceAccountName := cluster.Name + constants.SuffixBackupServiceAccount

	image := verifiedExecutorDigest
	if image == "" {
		image = cluster.Spec.Backup.ExecutorImage
	}
	if image == "" {
		image = constants.DefaultBackupImage()
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
				constants.LabelOpenBaoComponent: ComponentUpgradeSnapshot,
			},
			Annotations: map[string]string{
				AnnotationSnapshotPhase: phase,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster")),
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
						constants.LabelOpenBaoComponent: ComponentUpgradeSnapshot,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           serviceAccountName,
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
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: ptr.To(true),
								RunAsNonRoot:           ptr.To(true),
							},
							Env:          env,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	return job
}
