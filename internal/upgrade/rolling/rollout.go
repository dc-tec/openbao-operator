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
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/logging"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// performPodByPodUpgrade executes the rolling update, one pod at a time.
// Returns true when all pods have been upgraded.
// Returns false with nil error when waiting for a condition (caller should requeue).
func (m *Manager) performPodByPodUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *upgrade.Metrics) (bool, error) {
	if cluster.Status.Upgrade == nil {
		return false, fmt.Errorf("upgrade state is nil")
	}

	currentPartition := cluster.Status.Upgrade.CurrentPartition

	// If partition is 0, all pods have been updated
	if currentPartition == 0 {
		logger.Info("All pods have been updated")
		return true, nil
	}

	// The next pod to update is at ordinal (partition - 1)
	targetOrdinal := currentPartition - 1
	podName := fmt.Sprintf("%s-%d", cluster.Name, targetOrdinal)

	logger.Info("Processing pod for upgrade",
		"pod", podName,
		"ordinal", targetOrdinal,
		"partition", currentPartition)

	podStartTime := time.Now()

	leaderPodName, err := m.currentLeaderPodByLabel(ctx, cluster)
	if err != nil {
		logger.Info("Unable to determine current leader from pod labels; attempting safe step-down", "error", err)
	}

	// Step-down leader if needed (level-triggered)
	if leaderPodName == "" || leaderPodName == podName {
		logger.Info("Initiating leader step-down before updating pod", "pod", podName, "currentLeader", leaderPodName)
		stepDownComplete, err := m.stepDownLeader(ctx, logger, cluster, podName, metrics)
		if err != nil {
			return false, err
		}
		if !stepDownComplete {
			// Step-down in progress, requeue
			return false, nil
		}
	}

	// Decrement partition to allow this pod to update
	newPartition := currentPartition - 1
	if err := m.setStatefulSetPartition(ctx, cluster, newPartition); err != nil {
		return false, fmt.Errorf("failed to update partition: %w", err)
	}

	// Check pod readiness (level-triggered)
	podReady, err := m.waitForPodReady(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !podReady {
		// Pod not ready yet, requeue
		return false, nil
	}

	// Check pod health (level-triggered)
	podHealthy, err := m.waitForPodHealthy(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !podHealthy {
		// Pod not healthy yet, requeue
		return false, nil
	}

	// Update progress
	upgrade.SetUpgradeProgress(&cluster.Status, newPartition, targetOrdinal, cluster.Spec.Replicas, cluster.Generation)

	// Record pod upgrade duration
	podDuration := time.Since(podStartTime).Seconds()
	metrics.RecordPodDuration(podDuration, podName)
	metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
	metrics.SetPartition(newPartition)

	logger.Info("Pod upgrade completed",
		"pod", podName,
		"duration", podDuration,
		"remainingPartition", newPartition)

	// Check if there are more pods to update
	if newPartition > 0 {
		return false, nil
	}

	return true, nil
}

// currentLeaderPodByLabel returns the pod name labeled as the current leader, if available.
// Returns an empty string if no leader label is observed.
func (m *Manager) currentLeaderPodByLabel(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	podList := &corev1.PodList{}
	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelAppInstance: cluster.Name,
			constants.LabelAppName:     constants.LabelValueAppNameOpenBao,
		}),
	); err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	leaders := make([]string, 0, 1)
	for i := range podList.Items {
		pod := &podList.Items[i]
		leader, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil || !present {
			continue
		}
		if leader {
			leaders = append(leaders, pod.Name)
		}
	}

	switch len(leaders) {
	case 0:
		return "", nil
	case 1:
		return leaders[0], nil
	default:
		return "", fmt.Errorf("multiple leaders detected via pod labels (%d)", len(leaders))
	}
}

// stepDownLeader performs a leader step-down check using level-triggered semantics.
// Instead of blocking with a ticker loop, it checks the condition once and returns
// a result indicating whether to requeue.
//
// Returns:
//   - (true, nil) if step-down is complete
//   - (false, nil) if step-down is in progress (caller should requeue)
//   - (false, error) if step-down failed fatally
func (m *Manager) stepDownLeader(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string, metrics *upgrade.Metrics) (bool, error) {
	// Record step-down attempt (only on first call, tracked by LastStepDownTime)
	if cluster.Status.Upgrade == nil || cluster.Status.Upgrade.LastStepDownTime == nil {
		metrics.IncrementStepDownTotal()

		// Audit log: Leader step-down operation
		logging.LogAuditEvent(logger, "StepDown", map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
			"target_version":    cluster.Status.Upgrade.TargetVersion,
			"from_version":      cluster.Status.Upgrade.FromVersion,
		})
	}

	// Check if we've exceeded the timeout based on upgrade start time
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > upgrade.DefaultStepDownTimeout {
			metrics.IncrementStepDownFailures()
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonStepDownTimeout,
				fmt.Sprintf(upgrade.MessageStepDownTimeout, upgrade.DefaultStepDownTimeout),
				cluster.Generation)
			return false, fmt.Errorf("step-down timeout: exceeded %v", upgrade.DefaultStepDownTimeout)
		}
	}

	// Ensure step-down Job exists/is running
	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		upgrade.ExecutorActionRollingStepDownLeader,
		podName,
		"",
		"",
		m.clientConfig,
		m.operatorImageVerifier,
		m.Platform,
	)
	if err != nil {
		return false, fmt.Errorf("failed to ensure step-down Job: %w", err)
	}
	if result.Failed {
		metrics.IncrementStepDownFailures()
		return false, fmt.Errorf("step-down Job %s failed", result.Name)
	}
	if result.Running {
		logger.V(1).Info("Step-down job still running", "pod", podName)
		return false, nil // Requeue
	}

	// Job succeeded - check if leadership has transferred
	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      podName,
	}, pod); err != nil {
		logger.V(1).Info("Error getting pod after step-down", "error", err)
		return false, nil // Requeue
	}

	stillLeader, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
	if err != nil {
		logger.V(1).Info("Invalid OpenBao leader label value after step-down", "error", err)
		return false, nil // Requeue
	}

	if present && !stillLeader {
		logger.Info("Leadership transferred successfully", "previousLeader", podName)
		upgrade.SetStepDownPerformed(&cluster.Status)
		logging.LogAuditEvent(logger, "StepDownCompleted", map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
		})
		return true, nil
	}

	// If the previous leader label is missing, treat transfer as complete if we
	// can observe a different pod labeled as leader.
	if !present {
		podList := &corev1.PodList{}
		if err := m.client.List(ctx, podList,
			client.InNamespace(cluster.Namespace),
			client.MatchingLabels(map[string]string{
				constants.LabelAppInstance: cluster.Name,
				constants.LabelAppName:     constants.LabelValueAppNameOpenBao,
			}),
		); err != nil {
			logger.V(1).Info("Error listing pods while waiting for leader transfer", "error", err)
			return false, nil // Requeue
		}

		for i := range podList.Items {
			candidate := &podList.Items[i]
			if candidate.Name == podName {
				continue
			}
			leader, leaderPresent, err := openbaoapi.ParseBoolLabel(candidate.Labels, openbaoapi.LabelActive)
			if err != nil || !leaderPresent {
				continue
			}
			if leader {
				logger.Info("Leadership transferred successfully", "previousLeader", podName, "newLeader", candidate.Name)
				upgrade.SetStepDownPerformed(&cluster.Status)
				logging.LogAuditEvent(logger, "StepDownCompleted", map[string]string{
					"cluster_namespace": cluster.Namespace,
					"cluster_name":      cluster.Name,
					"pod":               podName,
				})
				return true, nil
			}
		}
	}

	logger.V(1).Info("Waiting for leadership transfer", "pod", podName)
	return false, nil // Requeue
}

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
				fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout),
				cluster.Generation)
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
				fmt.Sprintf(upgrade.MessageHealthCheckFailed, podName, "timeout"),
				cluster.Generation)
			return false, fmt.Errorf("OpenBao health check timeout for pod %s", podName)
		}
	}

	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		// CA cert not available yet, requeue
		logger.V(1).Info("CA certificate not available yet", "error", err)
		return false, nil // Requeue
	}

	podURL := m.getPodURL(cluster, podName)
	apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
		ClusterKey: fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name),
		BaseURL:    podURL,
		CACert:     caCert,
	})
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
