package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) ensureRollbackExecutorJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	action ExecutorAction,
	runningMessage string,
	failureMessage string,
	onFailed func(logr.Logger, *openbaov1alpha1.OpenBaoCluster, string),
) (phaseOutcome, bool, error) {
	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision

	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.reader,
		m.scheme,
		logger,
		cluster,
		action,
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
		logger.Info(runningMessage, "job", result.Name)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	if result.Failed {
		logger.Info(failureMessage, "job", result.Name)
		onFailed(logger, cluster, result.Name)
		return hold(), true, nil
	}

	return phaseOutcome{}, false, nil
}

func (m *Manager) ensureRollbackConsensusRepaired(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	return m.ensureRollbackExecutorJob(
		ctx,
		logger,
		cluster,
		ActionRepairConsensus,
		"Rollback job in progress: repairing consensus",
		"Rollback consensus repair job failed; entering break glass mode",
		m.enterBreakGlassRollbackConsensusRepairFailed,
	)
}

func (m *Manager) ensureBlueLeaderDuringRollback(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	bluePods, err := m.getPodsByRevision(ctx, cluster, cluster.Status.BlueGreen.BlueRevision)
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
	return m.ensureRollbackExecutorJob(
		ctx,
		logger,
		cluster,
		ActionRemoveGreenPeers,
		"Rollback job in progress: removing Green peers",
		"Rollback cleanup peer-removal job failed; entering break glass mode",
		m.enterBreakGlassRollbackCleanupPeerRemovalFailed,
	)
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

// handlePhaseRollbackCleanup retires discarded Green workloads and data after rollback.
func (m *Manager) handlePhaseRollbackCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	if outcome, waiting, err := m.ensureGreenPeersRemovedDuringRollbackCleanup(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	if outcome, waiting, err := m.ensureBlueLeaderDuringRollback(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}
	retired, err := m.retireRollbackGreenData(ctx, cluster)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to retire discarded Green data: %w", err)
	}
	if !retired {
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	return m.completeBlueGreenRollback(ctx, logger, cluster, cluster.Status.BlueGreen.RollbackReason)
}
