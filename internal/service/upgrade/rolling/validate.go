package rolling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// detectUpgradeState determines whether an upgrade is needed or if we're resuming one.
func (m *Manager) detectUpgradeState(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (upgradeNeeded bool, resumeUpgrade bool) {
	if upgrade.RetryRequestPending(cluster) &&
		(cluster.Status.Upgrade == nil ||
			strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason) == "" ||
			cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion) {
		retryRequest := upgrade.RetryRequestValue(cluster)
		upgrade.MarkRetryRequestHandled(&cluster.Status, retryRequest)
		logger.Info("Ignoring retry request because no failed rolling upgrade is waiting to resume",
			"retryRequest", retryRequest,
			"retryRequestField", upgrade.RequestRetryFieldPath)
	}

	// If upgrade is already in progress, we're resuming
	if cluster.Status.Upgrade != nil {
		if strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason) != "" {
			if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
				logger.Info("Failed upgrade target differs from spec; resuming to re-evaluate upgrade target",
					"failedTargetVersion", cluster.Status.Upgrade.TargetVersion,
					"specVersion", cluster.Spec.Version,
					"failureReason", cluster.Status.Upgrade.LastErrorReason)
				return false, true
			}

			if !upgrade.RetryRequestPending(cluster) {
				logger.Info("Upgrade is in failed state; waiting for manual retry request",
					"failureReason", cluster.Status.Upgrade.LastErrorReason,
					"failureMessage", cluster.Status.Upgrade.LastErrorMessage,
					"retryRequestField", upgrade.RequestRetryFieldPath)
				return false, false
			}

			logger.Info("Manual retry requested for failed upgrade",
				"retryRequest", upgrade.RetryRequestValue(cluster),
				"targetVersion", cluster.Status.Upgrade.TargetVersion,
				"currentPartition", cluster.Status.Upgrade.CurrentPartition)
			return false, true
		}

		logger.Info("Resuming in-progress upgrade",
			"fromVersion", cluster.Status.Upgrade.FromVersion,
			"targetVersion", cluster.Status.Upgrade.TargetVersion,
			"currentPartition", cluster.Status.Upgrade.CurrentPartition)
		return false, true
	}

	// If current version is empty, this is the first reconcile after initialization
	// Set it to spec.version and don't trigger an upgrade
	if cluster.Status.CurrentVersion == "" {
		logger.Info("Setting initial CurrentVersion from spec",
			"version", cluster.Spec.Version)
		// This is handled in the main controller status update
		return false, false
	}

	// Check if spec version differs from current version
	if cluster.Spec.Version == cluster.Status.CurrentVersion {
		logger.V(1).Info("No upgrade needed; versions match")
		return false, false
	}

	// Version mismatch - upgrade is needed
	logger.Info("Upgrade detected",
		"from", cluster.Status.CurrentVersion,
		"to", cluster.Spec.Version)
	return true, false
}

// validateUpgrade performs pre-upgrade validation checks.
func (m *Manager) validateUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := upgrade.ValidateUpgradeTargetVersion(logger, cluster.Status.CurrentVersion, cluster.Spec.Version); err != nil {
		return err
	}
	if err := upgrade.ValidateImageRefMatchesVersion(cluster.Spec.Version, cluster.Spec.Image); err != nil {
		return err
	}

	// New upgrades require a fully healthy cluster. In-progress upgrades use a
	// narrower gate so the target pod can be temporarily unavailable while the
	// controller waits for it to recover or time out.
	if cluster.Status.Upgrade != nil {
		if err := m.verifyResumeClusterHealth(ctx, logger, cluster); err != nil {
			return err
		}
		return nil
	}

	if err := m.verifyClusterHealth(ctx, logger, cluster); err != nil {
		return err
	}

	return nil
}

// verifyClusterHealth checks that the cluster is in a state suitable for upgrades.
func (m *Manager) verifyClusterHealth(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Get the StatefulSet
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, stsName, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("StatefulSet not found; cluster may not be fully initialized")
		}
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Verify all replicas are ready
	if sts.Status.ReadyReplicas != cluster.Spec.Replicas {
		return fmt.Errorf("not all replicas are ready (%d/%d)",
			sts.Status.ReadyReplicas, cluster.Spec.Replicas)
	}

	// Get cluster pods and verify health
	podList, err := m.getClusterPods(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to list cluster pods: %w", err)
	}

	if len(podList) != int(cluster.Spec.Replicas) {
		return fmt.Errorf("unexpected number of pods (%d/%d)",
			len(podList), cluster.Spec.Replicas)
	}

	// Verify quorum - at least (replicas/2)+1 must be healthy
	healthyCount, leaderCount, err := m.checkPodHealth(ctx, logger, cluster, podList)
	if err != nil {
		return fmt.Errorf("failed to check pod health: %w", err)
	}

	quorumRequired := (cluster.Spec.Replicas / 2) + 1
	if healthyCount < int(quorumRequired) {
		return fmt.Errorf("cluster has lost quorum (%d/%d healthy, need %d)",
			healthyCount, cluster.Spec.Replicas, quorumRequired)
	}

	// Verify single leader
	if leaderCount == 0 {
		return fmt.Errorf("no leader found in cluster")
	}
	if leaderCount > 1 {
		return fmt.Errorf("multiple leaders detected (%d); possible split-brain", leaderCount)
	}

	logger.Info("Cluster health verified",
		"healthyPods", healthyCount,
		"totalPods", cluster.Spec.Replicas,
		"leaderCount", leaderCount)

	return nil
}

// verifyResumeClusterHealth checks the minimum cluster safety required to
// continue an in-progress rolling upgrade. Unlike verifyClusterHealth, it does
// not require every replica to be ready because the currently updating pod may
// legitimately be unavailable while the rollout is in flight.
func (m *Manager) verifyResumeClusterHealth(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, stsName, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("StatefulSet not found; cluster may not be fully initialized")
		}
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	targetPodName := ""
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.CurrentPartition > 0 {
		targetPodName = fmt.Sprintf("%s-%d", cluster.Name, cluster.Status.Upgrade.CurrentPartition-1)
	}

	quorumRequired := (cluster.Spec.Replicas / 2) + 1
	if sts.Status.ReadyReplicas < quorumRequired {
		if err := markResumeUpgradeTimeout(cluster, targetPodName); err != nil {
			return err
		}
		return fmt.Errorf("rolling upgrade cannot continue without quorum-ready replicas (%d/%d ready, need %d)",
			sts.Status.ReadyReplicas, cluster.Spec.Replicas, quorumRequired)
	}

	podList, err := m.getClusterPods(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to list cluster pods: %w", err)
	}
	if len(podList) < int(quorumRequired) {
		if err := markResumeUpgradeTimeout(cluster, targetPodName); err != nil {
			return err
		}
		return fmt.Errorf("rolling upgrade cannot continue with too few cluster pods (%d/%d, need at least %d)",
			len(podList), cluster.Spec.Replicas, quorumRequired)
	}

	if err := m.verifyNonTargetPodsReadyAndHealthy(ctx, logger, cluster, podList, targetPodName); err != nil {
		return err
	}

	healthyCount, leaderCount, err := m.checkPodHealth(ctx, logger, cluster, podList)
	if err != nil {
		return fmt.Errorf("failed to check pod health: %w", err)
	}
	if healthyCount < int(quorumRequired) {
		return fmt.Errorf("cluster has lost quorum (%d/%d healthy, need %d)",
			healthyCount, cluster.Spec.Replicas, quorumRequired)
	}

	if leaderCount == 0 {
		return fmt.Errorf("no leader found in cluster")
	}
	if leaderCount > 1 {
		return fmt.Errorf("multiple leaders detected (%d); possible split-brain", leaderCount)
	}

	logger.Info("Rolling upgrade resume health verified",
		"healthyPods", healthyCount,
		"readyReplicas", sts.Status.ReadyReplicas,
		"totalPods", cluster.Spec.Replicas,
		"targetPod", targetPodName,
		"leaderCount", leaderCount)

	return nil
}

func markResumeUpgradeTimeout(cluster *openbaov1alpha1.OpenBaoCluster, podName string) error {
	if cluster == nil || cluster.Status.Upgrade == nil || cluster.Status.Upgrade.StartedAt == nil {
		return nil
	}
	if time.Since(cluster.Status.Upgrade.StartedAt.Time) <= upgrade.DefaultPodReadyTimeout {
		return nil
	}

	if strings.TrimSpace(podName) == "" {
		podName = "upgrade-target"
	}
	upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonPodNotReady,
		fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout))
	return fmt.Errorf("pod %s did not become ready within %v", podName, upgrade.DefaultPodReadyTimeout)
}

func (m *Manager) verifyNonTargetPodsReadyAndHealthy(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pods []corev1.Pod,
	targetPodName string,
) error {
	podsByName := make(map[string]*corev1.Pod, len(pods))
	for i := range pods {
		pod := &pods[i]
		podsByName[pod.Name] = pod
	}

	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to get CA certificate: %w", err)
	}

	for ordinal := int32(0); ordinal < cluster.Spec.Replicas; ordinal++ {
		podName := fmt.Sprintf("%s-%d", cluster.Name, ordinal)
		if podName == targetPodName {
			continue
		}

		pod := podsByName[podName]
		if pod == nil {
			return fmt.Errorf("rolling upgrade cannot continue while non-target pod %s is missing; current target is %s", podName, targetPodName)
		}
		if !isPodReady(pod) {
			return fmt.Errorf("rolling upgrade cannot continue while non-target pod %s is not ready; current target is %s", podName, targetPodName)
		}

		apiClient, err := m.newPodClient(cluster, podName, caCert)
		if err != nil {
			return fmt.Errorf("rolling upgrade cannot continue while non-target pod %s is unavailable: %w", podName, err)
		}

		healthy, err := apiClient.IsHealthy(ctx)
		if err != nil {
			logger.V(1).Info("Non-target pod health check failed during rolling resume validation", "pod", podName, "error", err)
			return fmt.Errorf("rolling upgrade cannot continue while non-target pod %s is unhealthy: %w", podName, err)
		}
		if !healthy {
			return fmt.Errorf("rolling upgrade cannot continue while non-target pod %s is unhealthy; current target is %s", podName, targetPodName)
		}
	}

	return nil
}

// checkPodHealth queries each pod's health status and returns counts.
func (m *Manager) checkPodHealth(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (healthyCount, leaderCount int, err error) {
	// Get CA cert for TLS connections
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get CA certificate: %w", err)
	}

	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		apiClient, err := m.newPodClient(cluster, pod.Name, caCert)
		if err != nil {
			logger.V(1).Info("Failed to create client for pod", "pod", pod.Name, "error", err)
			continue
		}

		healthy, err := apiClient.IsHealthy(ctx)
		if err != nil {
			logger.V(1).Info("Health check failed for pod", "pod", pod.Name, "error", err)
			continue
		}

		if healthy {
			healthyCount++
		}

		isLeader, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelActive)
		if err != nil {
			logger.V(1).Info("Invalid OpenBao leader label value", "pod", pod.Name, "error", err)
			continue
		}

		if !present {
			isLeader, err = apiClient.IsLeader(ctx)
			if err != nil {
				logger.V(1).Info("Leader check failed for pod", "pod", pod.Name, "error", err)
				continue
			}
		}

		if isLeader {
			leaderCount++
			cluster.Status.ActiveLeader = pod.Name
		}
	}

	return healthyCount, leaderCount, nil
}
