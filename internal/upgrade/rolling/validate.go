package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// detectUpgradeState determines whether an upgrade is needed or if we're resuming one.
func (m *Manager) detectUpgradeState(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (upgradeNeeded bool, resumeUpgrade bool) {
	// If upgrade is already in progress, we're resuming
	if cluster.Status.Upgrade != nil {
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
	// Validate target version format
	if err := upgrade.ValidateVersion(cluster.Spec.Version); err != nil {
		return fmt.Errorf("invalid target version: %w", err)
	}

	// Skip version comparison if this is resuming an upgrade or if no current version
	if cluster.Status.Upgrade == nil && cluster.Status.CurrentVersion != "" {
		// Check for downgrade
		if upgrade.IsDowngrade(cluster.Status.CurrentVersion, cluster.Spec.Version) {
			logger.Info("Downgrade detected and blocked",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
			return fmt.Errorf("downgrade from %s to %s is not allowed",
				cluster.Status.CurrentVersion, cluster.Spec.Version)
		}

		// Log warning for minor version skips or major upgrades
		change, _ := upgrade.CompareVersions(cluster.Status.CurrentVersion, cluster.Spec.Version)
		if change == upgrade.VersionChangeMajor {
			logger.Info("Major version upgrade detected; proceed with caution",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
		}
		if upgrade.IsSkipMinorUpgrade(cluster.Status.CurrentVersion, cluster.Spec.Version) {
			logger.Info("Minor version skip detected; some intermediate versions may be skipped",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
		}
	}

	// Verify cluster health
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

		podURL := m.getPodURL(cluster, pod.Name)
		apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
			ClusterKey: fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name),
			BaseURL:    podURL,
			CACert:     caCert,
		})
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

		isLeader, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
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
