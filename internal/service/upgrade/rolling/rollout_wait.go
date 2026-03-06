package rolling

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// setStatefulSetPartition updates the StatefulSet's partition value using strategic merge patch.
// We use MergeFrom instead of SSA because StatefulSet validation requires all required fields
// (selector, serviceName, template labels) to be present in SSA patches, but MergeFrom only
// sends the diff and doesn't have this limitation.
func (m *Manager) setStatefulSetPartition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, partition int32) error {
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	if err := m.client.Get(ctx, stsName, sts); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Create a patch that only updates the partition field.
	// We use client.MergeFrom instead of Server-Side Apply (SSA) because SSA requires
	// all required StatefulSet fields (selector, serviceName, template labels) to be present,
	// which causes validation errors. MergeFrom generates a strategic merge patch that only
	// touches the modified fields.
	newSts := sts.DeepCopy()
	newSts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	newSts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: &partition,
	}

	// Patch using MergeFrom to send only the differences
	if err := m.client.Patch(ctx, newSts, client.MergeFrom(sts)); err != nil {
		return fmt.Errorf("failed to update StatefulSet partition: %w", err)
	}

	return nil
}

// waitForPodRevisionUpdated checks whether a pod has rolled to the StatefulSet update revision.
// Returns:
//   - (true, nil) if the pod revision matches StatefulSet UpdateRevision
//   - (false, nil) if not updated yet (caller should requeue)
//   - (false, error) if timeout exceeded or fatal error
func (m *Manager) waitForPodRevisionUpdated(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > upgrade.DefaultPodReadyTimeout {
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonPodNotReady,
				fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout))
			return false, fmt.Errorf("pod %s did not roll to update revision within %v", podName, upgrade.DefaultPodReadyTimeout)
		}
	}

	sts := &appsv1.StatefulSet{}
	stsKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, stsKey, sts); err != nil {
		return false, fmt.Errorf("failed to get StatefulSet while checking pod revision: %w", err)
	}

	targetRevision := sts.Status.UpdateRevision
	if targetRevision == "" {
		logger.V(1).Info("StatefulSet update revision not set yet; waiting")
		return false, nil
	}

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

	podRevision := pod.Labels[appsv1.StatefulSetRevisionLabel]
	if podRevision != targetRevision {
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
	// Check timeout based on when the upgrade started
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > upgrade.DefaultPodReadyTimeout {
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonPodNotReady,
				fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout))
			return false, fmt.Errorf("pod %s did not become ready within %v", podName, upgrade.DefaultPodReadyTimeout)
		}
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
	// Check timeout based on when the upgrade started - health check should complete
	// within a reasonable window after the pod becomes ready
	// We use DefaultPodReadyTimeout + DefaultHealthCheckTimeout as total budget
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > upgrade.DefaultPodReadyTimeout+upgrade.DefaultHealthCheckTimeout {
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonHealthCheckFailed,
				fmt.Sprintf(upgrade.MessageHealthCheckFailed, podName, "timeout"))
			return false, fmt.Errorf("OpenBao health check timeout for pod %s", podName)
		}
	}

	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		// CA cert not available yet, requeue
		logger.V(1).Info("CA certificate not available yet", "error", err)
		return false, nil // Requeue
	}

	apiClient, err := m.newPodClient(cluster, podName, caCert)
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
