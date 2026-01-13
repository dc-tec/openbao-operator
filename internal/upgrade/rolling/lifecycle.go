package rolling

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// initializeUpgrade sets up the upgrade state and locks the StatefulSet partition.
func (m *Manager) initializeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	fromVersion := cluster.Status.CurrentVersion
	toVersion := cluster.Spec.Version

	logger.Info("Initializing upgrade",
		"from", fromVersion,
		"to", toVersion,
		"replicas", cluster.Spec.Replicas)

	// Set upgrade state
	upgrade.SetUpgradeStarted(&cluster.Status, fromVersion, toVersion, cluster.Spec.Replicas, cluster.Generation)

	// Lock StatefulSet by setting partition to replicas (prevents all updates)
	if err := m.setStatefulSetPartition(ctx, cluster, cluster.Spec.Replicas); err != nil {
		return fmt.Errorf("failed to lock StatefulSet partition: %w", err)
	}

	// Update status using SSA
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status after initializing upgrade: %w", err)
	}

	logger.Info("Upgrade initialized; StatefulSet partition locked",
		"partition", cluster.Spec.Replicas)

	return nil
}

// finalizeUpgrade completes the upgrade process.
func (m *Manager) finalizeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *upgrade.Metrics) error {
	var upgradeDuration float64
	var fromVersion string

	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		upgradeDuration = time.Since(cluster.Status.Upgrade.StartedAt.Time).Seconds()
		fromVersion = cluster.Status.Upgrade.FromVersion
	}

	// Mark upgrade complete
	upgrade.SetUpgradeComplete(&cluster.Status, cluster.Spec.Version, cluster.Generation)

	// Update status using SSA
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status after completing upgrade: %w", err)
	}

	// Record metrics
	if upgradeDuration > 0 {
		metrics.RecordDuration(upgradeDuration, fromVersion, cluster.Spec.Version)
	}
	metrics.SetInProgress(false)
	metrics.SetStatus(upgrade.UpgradeStatusSuccess)
	metrics.SetPodsCompleted(0)
	metrics.SetTotalPods(0)
	metrics.SetPartition(0)

	logger.Info("Upgrade completed successfully",
		"version", cluster.Spec.Version,
		"duration", upgradeDuration)

	return nil
}
