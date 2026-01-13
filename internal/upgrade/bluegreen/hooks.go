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
	"github.com/dc-tec/openbao-operator/internal/constants"
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
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
					constants.LabelAppInstance:      cluster.Name,
					constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
					constants.LabelOpenBaoCluster:   cluster.Name,
					constants.LabelOpenBaoComponent: component,
				},
			},
			Spec: batchv1.JobSpec{
				BackoffLimit:            &backoffLimit,
				ActiveDeadlineSeconds:   ptr.To(int64(timeout)),
				TTLSecondsAfterFinished: ttlSeconds,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{
							{
								Name:    "validation",
								Image:   hook.Image,
								Command: hook.Command,
								Args:    hook.Args,
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
