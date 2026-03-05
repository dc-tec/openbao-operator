package rolling

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// performPodByPodUpgrade executes the rolling update, one pod at a time.
// Returns true when all pods have been upgraded.
// Returns false with nil error when waiting for a condition (caller should requeue).
func (m *Manager) performPodByPodUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *upgrade.Metrics) (bool, error) {
	if cluster.Status.Upgrade == nil {
		return false, fmt.Errorf("upgrade state is nil")
	}

	currentPartition := cluster.Status.Upgrade.CurrentPartition

	// If partition is 0, all pods have been updated.
	if currentPartition == 0 {
		logger.Info("All pods have been updated")
		return true, nil
	}

	// The next pod to update is at ordinal (partition - 1).
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

	// Step-down leader if needed (level-triggered).
	if leaderPodName == "" || leaderPodName == podName {
		logger.Info("Initiating leader step-down before updating pod", "pod", podName, "currentLeader", leaderPodName)
		stepDownComplete, err := m.stepDownLeader(ctx, logger, cluster, podName, metrics)
		if err != nil {
			return false, err
		}
		if !stepDownComplete {
			// Step-down in progress, requeue.
			return false, nil
		}
	}

	// Decrement partition to allow this pod to update.
	newPartition := currentPartition - 1
	if err := m.setStatefulSetPartition(ctx, cluster, newPartition); err != nil {
		return false, fmt.Errorf("failed to update partition: %w", err)
	}

	// Check that the target pod has actually rolled to StatefulSet UpdateRevision.
	revisionUpdated, err := m.waitForPodRevisionUpdated(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !revisionUpdated {
		return false, nil // Requeue
	}

	// Check pod readiness (level-triggered).
	podReady, err := m.waitForPodReady(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !podReady {
		// Pod not ready yet, requeue.
		return false, nil
	}

	// Check pod health (level-triggered).
	podHealthy, err := m.waitForPodHealthy(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !podHealthy {
		// Pod not healthy yet, requeue.
		return false, nil
	}

	// Update progress.
	upgrade.SetUpgradeProgress(&cluster.Status, newPartition, targetOrdinal)

	// Record pod upgrade duration.
	podDuration := time.Since(podStartTime).Seconds()
	metrics.RecordPodDuration(podDuration, podName)
	metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
	metrics.SetPartition(newPartition)

	logger.Info("Pod upgrade completed",
		"pod", podName,
		"duration", podDuration,
		"remainingPartition", newPartition)

	// Check if there are more pods to update.
	if newPartition > 0 {
		return false, nil
	}

	return true, nil
}
