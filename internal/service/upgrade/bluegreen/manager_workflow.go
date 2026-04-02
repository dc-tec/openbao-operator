package bluegreen

import (
	"context"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
	"github.com/go-logr/logr"
)

// reconcileBlueGreen is the internal reconcile method that handles blue/green upgrades.
func (m *Manager) reconcileBlueGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) (result recon.Result, err error) {
	if !m.shouldReconcileBlueGreen(logger, cluster) {
		return recon.Result{}, nil
	}

	metrics := upgrade.NewMetrics(cluster.Namespace, cluster.Name)
	strategy := string(openbaov1alpha1.UpdateStrategyBlueGreen)

	if !cluster.Status.Initialized {
		metrics.SetInProgress(false)
		metrics.SetStatus(upgrade.UpgradeStatusNone)
		metrics.SetPodsCompleted(0)
		metrics.SetTotalPods(0)
		metrics.SetPartition(0)
		logger.Info("Cluster not initialized; skipping blue/green upgrade reconciliation")
		return requeueStandard(), nil
	}

	initialPhase := core.CurrentBlueGreenPhase(cluster)
	initialRollbackSet := core.IsBlueGreenRollbackSet(cluster)

	defer m.finalizeBlueGreenMetrics(metrics, strategy, cluster, initialPhase, initialRollbackSet)

	if result, done, err := m.prepareBlueGreenReconcile(ctx, logger, cluster); done || err != nil {
		return result, err
	}

	logger.Info("Upgrade detected; CurrentVersion differs from Spec.Version",
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version)

	result, err = m.executeStateMachine(ctx, logger, cluster, verifiedImageDigest)
	return result, err
}
