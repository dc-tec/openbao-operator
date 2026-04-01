package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) ensureRollbackConsensusRepaired(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision

	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionRepairConsensus,
		rollbackRunID(cluster),
		blueRevision,
		greenRevision,
		m.clientConfig,
		m.operatorImageVerifier,
		m.Platform,
	)
	if err != nil {
		return phaseOutcome{}, true, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: repairing consensus", "job", result.Name)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	if result.Failed {
		logger.Info("Rollback consensus repair job failed; entering break glass mode", "job", result.Name)
		m.enterBreakGlassRollbackConsensusRepairFailed(logger, cluster, result.Name)
		return hold(), true, nil
	}

	return phaseOutcome{}, false, nil
}

func (m *Manager) ensureBlueLeaderDuringRollback(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	bluePods, err := m.getBluePods(ctx, cluster, cluster.Status.BlueGreen.BlueRevision)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("failed to get Blue pods: %w", err)
	}

	leaderPod, source, ok := m.clusterOps.FindLeaderPod(ctx, logger, cluster, bluePods)
	if !ok {
		logger.Info("Blue leader not yet elected during rollback, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}

	logger.Info("Blue leader confirmed during rollback", "pod", leaderPod, "source", source)
	return phaseOutcome{}, false, nil
}

func (m *Manager) ensureGreenPeersRemovedDuringRollbackCleanup(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision

	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionRemoveGreenPeers,
		rollbackRunID(cluster),
		blueRevision,
		greenRevision,
		m.clientConfig,
		m.operatorImageVerifier,
		m.Platform,
	)
	if err != nil {
		return phaseOutcome{}, true, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: removing Green peers", "job", result.Name)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	if result.Failed {
		logger.Info("Rollback cleanup peer-removal job failed; entering break glass mode", "job", result.Name)
		m.enterBreakGlassRollbackCleanupPeerRemovalFailed(logger, cluster, result.Name)
		return hold(), true, nil
	}

	return phaseOutcome{}, false, nil
}

func (m *Manager) ensureGreenPodsRemovedDuringRollback(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	greenPods, err := m.getGreenPods(ctx, cluster, cluster.Status.BlueGreen.GreenRevision)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("failed to check Green pods: %w", err)
	}

	activeGreenPods := countActivePods(greenPods)
	if activeGreenPods > 0 {
		logger.Info("Green pods still exist during rollback cleanup, waiting", "count", activeGreenPods)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}

	return phaseOutcome{}, false, nil
}

// handlePhaseRollingBack orchestrates the rollback sequence.
func (m *Manager) handlePhaseRollingBack(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	if outcome, waiting, err := m.ensureRollbackConsensusRepaired(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}
	if outcome, waiting, err := m.ensureBlueLeaderDuringRollback(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}
	return advance(openbaov1alpha1.PhaseRollbackCleanup), nil
}

// handlePhaseRollbackCleanup removes Green StatefulSet after rollback.
func (m *Manager) handlePhaseRollbackCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	if outcome, waiting, err := m.ensureGreenPeersRemovedDuringRollbackCleanup(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to cleanup Green StatefulSet during rollback: %w", err)
	}
	if outcome, waiting, err := m.ensureGreenPodsRemovedDuringRollback(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	return m.completeBlueGreenRollback(ctx, logger, cluster, cluster.Status.BlueGreen.RollbackReason)
}
