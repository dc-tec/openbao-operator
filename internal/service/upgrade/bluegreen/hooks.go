package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (m *Manager) ensureValidationHookJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	component string,
	hook *openbaov1alpha1.ValidationHookConfig,
) (*JobResult, error) {
	if hook == nil {
		return nil, fmt.Errorf("hook is required")
	}
	if hook.Image == "" {
		return nil, fmt.Errorf("hook.image is required")
	}

	timeout := int32(300)
	if hook.TimeoutSeconds != nil {
		timeout = *hook.TimeoutSeconds
	}
	backoffLimit := int32(0)
	ttlSeconds := ptr.To(int32(jobTTLSeconds))

	return ensureJob(ctx, m.client, m.scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		image := hook.Image
		verifiedDigest, err := m.verifyOperatorImageDigest(
			ctx,
			logger,
			cluster,
			hook.Image,
			constants.ReasonValidationHookImageVerificationFailed,
			"Validation hook image verification failed",
		)
		if err != nil {
			return nil, err
		}
		if verifiedDigest != "" {
			image = verifiedDigest
		}

		jobLabels := map[string]string{
			constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
			constants.LabelAppInstance:      cluster.Name,
			constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
			constants.LabelOpenBaoCluster:   cluster.Name,
			constants.LabelOpenBaoComponent: component,
		}
		security.AddManagedWorkloadSecurityLabels(jobLabels, cluster)

		podTemplateLabels := map[string]string{
			constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
			constants.LabelAppInstance:      cluster.Name,
			constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
			constants.LabelOpenBaoCluster:   cluster.Name,
			constants.LabelOpenBaoComponent: component,
		}
		security.AddManagedWorkloadSecurityLabels(podTemplateLabels, cluster)

		podSecurityContext := &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
		if m.Platform != constants.PlatformOpenShift {
			podSecurityContext.RunAsUser = ptr.To(constants.UserNonRoot)
			podSecurityContext.RunAsGroup = ptr.To(constants.UserNonRoot)
		}

		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: cluster.Namespace,
				Labels:    jobLabels,
			},
			Spec: batchv1.JobSpec{
				BackoffLimit:            &backoffLimit,
				ActiveDeadlineSeconds:   ptr.To(int64(timeout)),
				TTLSecondsAfterFinished: ttlSeconds,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: podTemplateLabels,
					},
					Spec: corev1.PodSpec{
						AutomountServiceAccountToken: ptr.To(false),
						ImagePullSecrets:             cluster.Spec.ImagePullSecrets,
						RestartPolicy:                corev1.RestartPolicyNever,
						SecurityContext:              podSecurityContext,
						Containers: []corev1.Container{
							{
								Name:    "validation",
								Image:   image,
								Command: hook.Command,
								Args:    hook.Args,
								SecurityContext: &corev1.SecurityContext{
									AllowPrivilegeEscalation: ptr.To(false),
									Capabilities: &corev1.Capabilities{
										Drop: []corev1.Capability{"ALL"},
									},
									ReadOnlyRootFilesystem: ptr.To(true),
									RunAsNonRoot:           ptr.To(true),
								},
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
							},
						},
					},
				},
			},
		}, nil
	}, "component", component)
}

func (m *Manager) ensurePrePromotionHookJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	hook *openbaov1alpha1.ValidationHookConfig,
) (*JobResult, error) {
	return m.ensureValidationHookJob(ctx, logger, cluster, fmt.Sprintf("%s-validation-hook", cluster.Name), ComponentValidationHook, hook)
}
