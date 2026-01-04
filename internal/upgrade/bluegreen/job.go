package bluegreen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/kube"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

const (
	jobNamePrefix = "upgrade-"
	jobTTLSeconds = 3600 // 1 hour TTL for completed/failed jobs

	tokenVolumeName       = "openbao-token"
	tokenMountPath        = "/var/run/secrets/tokens" // #nosec G101 -- This is a mount path constant, not a credential
	tokenFileRelativePath = "openbao-token"
	tlsCAVolumeName       = "tls-ca"
)

// JobResult contains the status of an upgrade executor Job.
type JobResult struct {
	Name      string
	Exists    bool
	Succeeded bool
	Failed    bool
	Running   bool
}

type buildJobFunc func(jobName string) (*batchv1.Job, error)

func ensureJob(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	build buildJobFunc,
	createLogKeysAndValues ...any,
) (*JobResult, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if jobName == "" {
		return nil, fmt.Errorf("jobName is required")
	}
	if build == nil {
		return nil, fmt.Errorf("build function is required")
	}

	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}
	job := &batchv1.Job{}
	if err := c.Get(ctx, jobKey, job); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		built, err := build(jobName)
		if err != nil {
			return nil, err
		}

		if err := controllerutil.SetControllerReference(cluster, built, scheme); err != nil {
			return nil, fmt.Errorf("failed to set owner reference on Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		logger.Info("Creating Job", append([]any{"job", jobName}, createLogKeysAndValues...)...)
		if err := c.Create(ctx, built); err != nil {
			return nil, fmt.Errorf("failed to create Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		return &JobResult{
			Name:    jobName,
			Exists:  true,
			Running: true,
		}, nil
	}

	if kube.JobSucceeded(job) {
		return &JobResult{
			Name:      jobName,
			Exists:    true,
			Succeeded: true,
		}, nil
	}

	if kube.JobFailed(job) {
		return &JobResult{
			Name:   jobName,
			Exists: true,
			Failed: true,
		}, nil
	}

	return &JobResult{
		Name:    jobName,
		Exists:  true,
		Running: true,
	}, nil
}

func getJobStatus(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (*JobResult, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if jobName == "" {
		return nil, fmt.Errorf("jobName is required")
	}

	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}
	job := &batchv1.Job{}
	if err := c.Get(ctx, jobKey, job); err != nil {
		if apierrors.IsNotFound(err) {
			return &JobResult{Name: jobName, Exists: false}, nil
		}
		return nil, fmt.Errorf("failed to get Job %s/%s: %w", cluster.Namespace, jobName, err)
	}

	if kube.JobSucceeded(job) {
		return &JobResult{Name: jobName, Exists: true, Succeeded: true}, nil
	}
	if kube.JobFailed(job) {
		return &JobResult{Name: jobName, Exists: true, Failed: true}, nil
	}
	return &JobResult{Name: jobName, Exists: true, Running: true}, nil
}

// EnsureExecutorJob creates or checks the status of an upgrade executor Job.
func EnsureExecutorJob(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	action ExecutorAction,
	runID string,
	blueRevision string,
	greenRevision string,
) (*JobResult, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}

	jobName := executorJobName(cluster.Name, action, runID, blueRevision, greenRevision)
	return ensureJob(ctx, c, scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		verifiedExecutorDigest := ""
		if cluster.Spec.Upgrade != nil {
			executorImage := strings.TrimSpace(cluster.Spec.Upgrade.ExecutorImage)
			if executorImage != "" && cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled {
				verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
				defer cancel()

				digest, err := security.VerifyImageForCluster(verifyCtx, logger, c, cluster, executorImage)
				if err != nil {
					failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
					if failurePolicy == "" {
						failurePolicy = constants.ImageVerificationFailurePolicyBlock
					}
					if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
						return nil, fmt.Errorf("upgrade executor image verification failed (policy=Block): %w", err)
					}
					logger.Error(err, "Upgrade executor image verification failed but proceeding due to Warn policy", "image", executorImage)
				} else {
					verifiedExecutorDigest = digest
					logger.Info("Upgrade executor image verified successfully", "digest", digest)
				}
			}
		}

		job, err := buildExecutorJob(cluster, jobName, action, runID, blueRevision, greenRevision, verifiedExecutorDigest)
		if err != nil {
			return nil, fmt.Errorf("failed to build upgrade Job %s/%s: %w", cluster.Namespace, jobName, err)
		}
		return job, nil
	}, "action", action, "runID", runID)
}

func buildExecutorJob(
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	action ExecutorAction,
	runID string,
	blueRevision string,
	greenRevision string,
	verifiedExecutorDigest string,
) (*batchv1.Job, error) {
	if cluster.Spec.Upgrade == nil {
		return nil, fmt.Errorf("spec.upgrade is required for upgrade Jobs")
	}

	image := verifiedExecutorDigest
	if image == "" {
		image = strings.TrimSpace(cluster.Spec.Upgrade.ExecutorImage)
	}
	if image == "" {
		return nil, fmt.Errorf("spec.upgrade.executorImage is required for upgrade Jobs")
	}

	jwtRole := strings.TrimSpace(cluster.Spec.Upgrade.JWTAuthRole)
	if jwtRole == "" {
		return nil, fmt.Errorf("spec.upgrade.jwtAuthRole is required for upgrade Jobs")
	}

	env := []corev1.EnvVar{
		{Name: constants.EnvClusterNamespace, Value: cluster.Namespace},
		{Name: constants.EnvClusterName, Value: cluster.Name},
		{Name: constants.EnvClusterReplicas, Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: constants.EnvUpgradeAction, Value: string(action)},
		{Name: constants.EnvUpgradeJWTAuthRole, Value: jwtRole},
	}

	if blueRevision != "" {
		env = append(env, corev1.EnvVar{Name: constants.EnvUpgradeBlueRevision, Value: blueRevision})
	}
	if greenRevision != "" {
		env = append(env, corev1.EnvVar{Name: constants.EnvUpgradeGreenRevision, Value: greenRevision})
	}

	backoffLimit := int32(0)
	ttlSecondsAfterFinished := int32(jobTTLSeconds)

	tokenFileMode := int32(0400) // Owner read-only

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
			},
			Annotations: buildExecutorJobAnnotations(action, runID),
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
						constants.LabelOpenBaoComponent: upgrade.ComponentUpgrade,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           cluster.Name + constants.SuffixUpgradeServiceAccount,
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
							Name:  "upgrade-executor",
							Image: image,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: ptr.To(true),
								RunAsNonRoot:           ptr.To(true),
							},
							Env: env,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      tlsCAVolumeName,
									MountPath: constants.PathTLS,
									ReadOnly:  true,
								},
								{
									Name:      tokenVolumeName,
									MountPath: tokenMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: tlsCAVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: cluster.Name + constants.SuffixTLSCA,
								},
							},
						},
						{
							Name: tokenVolumeName,
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
												Path:              tokenFileRelativePath,
												ExpirationSeconds: ptr.To(int64(3600)),
												Audience:          constants.TokenAudienceOpenBaoInternal,
											},
										},
									},
									DefaultMode: &tokenFileMode,
								},
							},
						},
					},
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

func buildExecutorJobAnnotations(action ExecutorAction, runID string) map[string]string {
	annotations := map[string]string{
		"openbao.org/upgrade-action": string(action),
	}
	if strings.TrimSpace(runID) != "" {
		annotations["openbao.org/upgrade-run-id"] = runID
	}
	return annotations
}

func executorJobName(clusterName string, action ExecutorAction, runID string, blueRevision, greenRevision string) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s", clusterName, action, runID, blueRevision, greenRevision)
	sum := sha256.Sum256([]byte(payload))
	suffix := hex.EncodeToString(sum[:])[:10]

	// Keep the name stable and within the 63-char DNS label limit.
	base := fmt.Sprintf("%s%s-%s", jobNamePrefix, clusterName, string(action))
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, "_", "-")

	maxBaseLen := 63 - 1 - len(suffix) // "-" + suffix
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
		base = strings.TrimRight(base, "-")
	}

	return fmt.Sprintf("%s-%s", base, suffix)
}

func preUpgradeSnapshotJobName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	blueRevision := ""
	if cluster.Status.BlueGreen != nil {
		blueRevision = cluster.Status.BlueGreen.BlueRevision
	}

	payload := fmt.Sprintf("%s|pre-upgrade-snapshot|%s|%s|%s|%d|%s",
		cluster.Name,
		cluster.Status.CurrentVersion,
		cluster.Spec.Version,
		cluster.Spec.Image,
		cluster.Spec.Replicas,
		blueRevision,
	)
	sum := sha256.Sum256([]byte(payload))
	suffix := hex.EncodeToString(sum[:])[:10]

	base := fmt.Sprintf("%s%s-preupgrade-snapshot", jobNamePrefix, cluster.Name)
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, "_", "-")

	maxBaseLen := 63 - 1 - len(suffix) // "-" + suffix
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
		base = strings.TrimRight(base, "-")
	}

	return fmt.Sprintf("%s-%s", base, suffix)
}
