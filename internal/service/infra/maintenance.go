package infra

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func (m *Manager) reconcileMaintenanceAnnotationsForPods(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, revision string) error {
	if cluster == nil {
		return nil
	}

	enabled := cluster.Spec.Maintenance != nil && cluster.Spec.Maintenance.Enabled

	var pods corev1.PodList
	if err := m.client.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelectorLabelsWithRevision(cluster, revision)),
	); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}

		original := pod.DeepCopy()
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}

		if enabled {
			pod.Annotations[constants.AnnotationMaintenance] = "true"
		} else {
			delete(pod.Annotations, constants.AnnotationMaintenance)
		}

		if reflect.DeepEqual(original.Annotations, pod.Annotations) {
			continue
		}

		if err := m.client.Patch(ctx, pod, client.MergeFrom(original)); err != nil {
			return fmt.Errorf("failed to patch pod %s/%s maintenance annotation: %w", pod.Namespace, pod.Name, err)
		}
		logger.V(1).Info("Patched pod maintenance annotation", "pod", pod.Name, "enabled", enabled)
	}

	return nil
}
