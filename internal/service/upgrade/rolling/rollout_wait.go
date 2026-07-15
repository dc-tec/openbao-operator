package rolling

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

// setStatefulSetPartition updates only the StatefulSet rolling-update partition.
// MergeFrom avoids the full-object validation burden of SSA for StatefulSets.
func (m *Manager) setStatefulSetPartition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, partition int32) error {
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      upgrade.StableVoterStatefulSetName(cluster),
	}

	if err := m.client.Get(ctx, stsName, sts); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	newSts := sts.DeepCopy()
	newSts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	newSts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: &partition,
	}

	if err := m.client.Patch(ctx, newSts, client.MergeFrom(sts)); err != nil {
		return fmt.Errorf("failed to update StatefulSet partition: %w", err)
	}

	return nil
}

// waitForPodRevisionUpdated checks whether a pod has rolled to the StatefulSet
// update revision.
func (m *Manager) waitForPodRevisionUpdated(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	if err := failUpgradeIfStartedTimeout(cluster, podRevisionTimeout(podName)); err != nil {
		return false, err
	}

	sts := &appsv1.StatefulSet{}
	stsKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      upgrade.StableVoterStatefulSetName(cluster),
	}
	if err := m.client.Get(ctx, stsKey, sts); err != nil {
		return false, fmt.Errorf("failed to get StatefulSet while checking pod revision: %w", err)
	}

	targetRevision := strings.TrimSpace(sts.Status.UpdateRevision)
	if targetRevision == "" {
		logger.V(1).Info("StatefulSet update revision not set yet; waiting")
		return false, nil
	}
	desiredImage := strings.TrimSpace(baoContainerImage(sts.Spec.Template.Spec.Containers))

	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      podName,
	}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Pod not found yet while waiting for revision update", "pod", podName)
			return false, nil
		}
		return false, fmt.Errorf("failed to get pod %s while checking revision: %w", podName, err)
	}

	podRevision := strings.TrimSpace(pod.Labels[appsv1.StatefulSetRevisionLabel])
	podImage := strings.TrimSpace(baoContainerImage(pod.Spec.Containers))
	if podRevision != targetRevision {
		// If the pod no longer matches the StatefulSet template, waiting alone can stall
		// forever after a failed retry because the StatefulSet controller does not replace
		// an already-existing stale pod on its own. Force a fresh recreate instead.
		if desiredImage != "" && podImage != desiredImage {
			if err := m.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to delete stale pod %s while waiting for revision update: %w", podName, err)
			}
			logger.Info("Deleted stale pod while waiting for revision update",
				"pod", podName,
				"currentRevision", podRevision,
				"targetRevision", targetRevision,
				"podImage", podImage,
				"desiredImage", desiredImage)
			return false, nil
		}

		logger.V(1).Info("Waiting for pod revision update",
			"pod", podName,
			"currentRevision", podRevision,
			"targetRevision", targetRevision)
		return false, nil
	}

	logger.Info("Pod revision updated", "pod", podName, "revision", targetRevision)
	return true, nil
}

// waitForPodReady checks if a pod is Ready using level-triggered semantics.
// Instead of blocking, it checks the condition once and returns the result.
//
// Returns:
//   - (true, nil) if pod is ready
//   - (false, nil) if pod is not ready yet (caller should requeue)
//   - (false, error) if timeout exceeded or fatal error
func (m *Manager) waitForPodReady(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	if err := failUpgradeIfStartedTimeout(cluster, podReadyTimeout(podName)); err != nil {
		return false, err
	}

	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      podName,
	}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Pod not found yet; waiting", "pod", podName)
			return false, nil // Requeue
		}
		return false, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	if isPodReady(pod) {
		logger.Info("Pod is ready", "pod", podName)
		return true, nil
	}

	logger.V(1).Info("Waiting for pod to become ready", "pod", podName, "phase", pod.Status.Phase)
	return false, nil // Requeue
}

// waitForPodHealthy checks if OpenBao is healthy on a pod using level-triggered semantics.
// Instead of blocking, it checks the condition once and returns the result.
//
// Returns:
//   - (true, nil) if pod is healthy
//   - (false, nil) if pod is not healthy yet (caller should requeue)
//   - (false, error) if timeout exceeded or fatal error
func (m *Manager) waitForPodHealthy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	if err := failUpgradeIfStartedTimeout(cluster, podHealthTimeout(podName)); err != nil {
		return false, err
	}

	caCert, err := raftops.LoadClusterCACert(ctx, m.client, cluster)
	if err != nil {
		// CA cert not available yet, requeue
		logger.V(1).Info("CA certificate not available yet", "error", err)
		return false, nil // Requeue
	}

	apiClient, err := raftops.NewClusterPodClient(cluster, podName, caCert, m.clientFactory, raftops.ClusterPodClientOptions{})
	if err != nil {
		// Wrap connection errors as transient and requeue
		if operatorerrors.IsTransientConnection(err) {
			logger.V(1).Info("Transient connection error creating client", "error", err)
			return false, nil // Requeue
		}
		return false, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	healthy, err := apiClient.IsHealthy(ctx)
	if err != nil {
		logger.V(1).Info("Health check error; will retry", "pod", podName, "error", err)
		return false, nil // Requeue
	}
	if healthy {
		logger.Info("OpenBao is healthy on pod", "pod", podName)
		return true, nil
	}

	logger.V(1).Info("Waiting for OpenBao to become healthy", "pod", podName)
	return false, nil // Requeue
}
