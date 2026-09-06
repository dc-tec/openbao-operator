package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func blueGreenPhaseString(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Status.BlueGreen == nil {
		return "nil"
	}
	return string(cluster.Status.BlueGreen.Phase)
}

func (m *Manager) handleManualRollbackRequest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, acknowledgements *upgrade.RequestAcknowledgements) (bool, recon.Result, error) {
	if !upgrade.RollbackRequestPending(cluster) {
		return false, recon.Result{}, nil
	}

	rollbackRequest := upgrade.RollbackRequestValue(cluster)

	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
		acknowledgements.Rollback = rollbackRequest
		logger.Info("Ignoring rollback request because no blue/green upgrade is active",
			"rollbackRequest", rollbackRequest,
			"rollbackRequestField", upgrade.RequestRollbackFieldPath)
		return false, recon.Result{}, nil
	}

	if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollingBack ||
		cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollbackCleanup {
		acknowledgements.Rollback = rollbackRequest
		logger.Info("Ignoring rollback request because rollback is already in progress",
			"rollbackRequest", rollbackRequest,
			"phase", cluster.Status.BlueGreen.Phase,
			"rollbackRequestField", upgrade.RequestRollbackFieldPath)
		return false, recon.Result{}, nil
	}

	logger.Info("Manual rollback requested",
		"rollbackRequest", rollbackRequest,
		"phase", cluster.Status.BlueGreen.Phase,
		"rollbackRequestField", upgrade.RequestRollbackFieldPath)

	if cluster.Status.BlueGreen.GreenRevision == "" {
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return false, recon.Result{}, fmt.Errorf("failed to abort upgrade via %s: %w", upgrade.RequestRollbackFieldPath, err)
		}
		acknowledgements.Rollback = rollbackRequest
		return true, recon.Result{}, nil
	}

	result, err := m.triggerRollback(logger, cluster, fmt.Sprintf("manual rollback request via %s", upgrade.RequestRollbackFieldPath))
	if err == nil {
		acknowledgements.Rollback = rollbackRequest
	}
	return true, result, err
}
