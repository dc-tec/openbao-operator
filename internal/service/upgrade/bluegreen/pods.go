package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// isPodReady checks if a pod is in Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// getPodsByRevision returns all pods belonging to the specified revision.
// This is a unified helper used for both Blue and Green pod lookup.
func (m *Manager) getPodsByRevision(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, rev string) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:     cluster.Name,
		constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
		constants.LabelOpenBaoRevision: rev,
	})

	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, fmt.Errorf("failed to list pods for revision %s: %w", rev, err)
	}

	return podList.Items, nil
}

// getGreenPods returns all pods belonging to the Green revision (convenience wrapper).
func (m *Manager) getGreenPods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, greenRevision string) ([]corev1.Pod, error) {
	return m.getPodsByRevision(ctx, cluster, greenRevision)
}

// getBluePods returns all pods belonging to the Blue revision (convenience wrapper).
func (m *Manager) getBluePods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, blueRevision string) ([]corev1.Pod, error) {
	return m.getPodsByRevision(ctx, cluster, blueRevision)
}

// cleanupGreenStatefulSet removes any lingering Green StatefulSet.
func (m *Manager) cleanupGreenStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.GreenRevision == "" {
		return nil
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	greenStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      fmt.Sprintf("%s-%s", cluster.Name, greenRevision),
	}, greenStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to get Green StatefulSet: %w", err)
	}

	if err := m.client.Delete(ctx, greenStatefulSet); err != nil {
		return fmt.Errorf("failed to delete Green StatefulSet: %w", err)
	}

	logger.Info("Cleaned up Green StatefulSet", "greenRevision", greenRevision)
	return nil
}
