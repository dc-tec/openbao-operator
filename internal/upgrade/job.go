package upgrade

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

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
)

const (
	upgradeJobNamePrefix = "upgrade-"
	upgradeJobTTLSeconds = 3600 // 1 hour TTL for completed/failed jobs

	upgradeTokenVolumeName       = "openbao-token"
	upgradeTokenMountPath        = "/var/run/secrets/tokens" // #nosec G101 -- This is a mount path constant, not a credential
	upgradeTokenFileRelativePath = "openbao-token"
	upgradeTLSCAVolumeName       = "tls-ca"
)

type executorJobResult struct {
	Name      string
	Succeeded bool
	Failed    bool
	Running   bool
}

func ensureUpgradeExecutorJob(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	action ExecutorAction,
	runID string,
	blueRevision string,
	greenRevision string,
) (*executorJobResult, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}

	jobName := upgradeExecutorJobName(cluster.Name, action, runID, blueRevision, greenRevision)
	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}

	job := &batchv1.Job{}
	if err := c.Get(ctx, jobKey, job); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get upgrade Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		job, err := buildUpgradeExecutorJob(cluster, jobName, action, runID, blueRevision, greenRevision)
		if err != nil {
			return nil, fmt.Errorf("failed to build upgrade Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		if err := controllerutil.SetControllerReference(cluster, job, scheme); err != nil {
			return nil, fmt.Errorf("failed to set owner reference on upgrade Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		logger.Info("Creating upgrade executor Job", "job", jobName, "action", action, "runID", runID)
		if err := c.Create(ctx, job); err != nil {
			return nil, fmt.Errorf("failed to create upgrade Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		return &executorJobResult{
			Name:    jobName,
			Running: true,
		}, nil
	}

	if job.Status.Succeeded > 0 {
		return &executorJobResult{
			Name:      jobName,
			Succeeded: true,
		}, nil
	}

	if job.Status.Failed > 0 {
		return &executorJobResult{
			Name:   jobName,
			Failed: true,
		}, nil
	}

	return &executorJobResult{
		Name:    jobName,
		Running: true,
	}, nil
}

func buildUpgradeExecutorJob(
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	action ExecutorAction,
	runID string,
	blueRevision string,
	greenRevision string,
) (*batchv1.Job, error) {
	if cluster.Spec.Upgrade == nil {
		return nil, fmt.Errorf("spec.upgrade is required for upgrade Jobs")
	}

	image := strings.TrimSpace(cluster.Spec.Upgrade.ExecutorImage)
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
	ttlSecondsAfterFinished := int32(upgradeJobTTLSeconds)

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
				constants.LabelOpenBaoComponent: ComponentUpgrade,
			},
			Annotations: buildUpgradeExecutorJobAnnotations(action, runID),
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
						constants.LabelOpenBaoComponent: ComponentUpgrade,
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
									Name:      upgradeTLSCAVolumeName,
									MountPath: constants.PathTLS,
									ReadOnly:  true,
								},
								{
									Name:      upgradeTokenVolumeName,
									MountPath: upgradeTokenMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: upgradeTLSCAVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: cluster.Name + constants.SuffixTLSCA,
								},
							},
						},
						{
							Name: upgradeTokenVolumeName,
							VolumeSource: corev1.VolumeSource{
								Projected: &corev1.ProjectedVolumeSource{
									Sources: []corev1.VolumeProjection{
										{
											ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
												Path:              upgradeTokenFileRelativePath,
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

func buildUpgradeExecutorJobAnnotations(action ExecutorAction, runID string) map[string]string {
	annotations := map[string]string{
		"openbao.org/upgrade-action": string(action),
	}
	if strings.TrimSpace(runID) != "" {
		annotations["openbao.org/upgrade-run-id"] = runID
	}
	return annotations
}

func upgradeExecutorJobName(clusterName string, action ExecutorAction, runID string, blueRevision, greenRevision string) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%s", clusterName, action, runID, blueRevision, greenRevision)
	sum := sha256.Sum256([]byte(payload))
	suffix := hex.EncodeToString(sum[:])[:10]

	// Keep the name stable and within the 63-char DNS label limit.
	base := fmt.Sprintf("%s%s-%s", upgradeJobNamePrefix, clusterName, string(action))
	base = strings.ToLower(base)
	base = strings.ReplaceAll(base, "_", "-")

	maxBaseLen := 63 - 1 - len(suffix) // "-" + suffix
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
		base = strings.TrimRight(base, "-")
	}

	return fmt.Sprintf("%s-%s", base, suffix)
}
