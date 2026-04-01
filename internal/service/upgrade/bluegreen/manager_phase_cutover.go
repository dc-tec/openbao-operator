package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// handlePhaseSyncing waits for Green nodes to catch up with Blue nodes.
func (m *Manager) handlePhaseSyncing(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	if outcome, waiting, err := m.ensureMinimumSyncDuration(logger, cluster); waiting || err != nil {
		return outcome, err
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionWaitGreenSynced, "job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	if outcome, waiting, err := m.ensurePrePromotionHookComplete(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	return m.decideSyncPromotion(logger, cluster)
}

// handlePhasePromoting promotes Green nodes to voters.
func (m *Manager) handlePhasePromoting(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionPromoteGreenVoters, "promotion job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	return advance(openbaov1alpha1.PhaseDemotingBlue), nil
}

// handlePhaseDemotingBlue demotes Blue nodes to non-voters and verifies Green becomes leader.
// After demotion, Blue nodes are no longer voters, so Green nodes (the only voters) will win any election.
func (m *Manager) handlePhaseDemotingBlue(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	metrics := m.markBlueGreenStepDown(logger, cluster)

	greenSnapshots, err := m.ensureGreenReadyForBlueDemotion(ctx, logger, cluster)
	if err != nil {
		return phaseOutcome{}, err
	}
	if greenSnapshots == nil {
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	previousLastJobFailure := cluster.Status.BlueGreen.LastJobFailure
	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionDemoteBlueNonVotersStepDown, "demotion job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		if cluster.Status.BlueGreen.LastJobFailure != "" && cluster.Status.BlueGreen.LastJobFailure != previousLastJobFailure {
			metrics.IncrementStepDownFailures()
		}
		return step.Outcome, nil
	}

	if outcome, waiting, err := m.waitForGreenLeaderAfterBlueDemotion(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	return advance(openbaov1alpha1.PhaseCleanup), nil
}

// handlePhaseCleanup ejects Blue nodes from Raft and deletes the Blue StatefulSet.
// This is the point of no return; after this, rollback is not possible.
func (m *Manager) handlePhaseCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	if outcome, waiting, err := m.ensureGreenReadyForBlueCleanup(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionRemoveBluePeers, "cleanup peer removal job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	if outcome, waiting, err := m.ensureBlueStatefulSetDeleted(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}
	if outcome, waiting, err := m.ensureBluePodsTerminated(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	return m.completeBlueGreenUpgrade(ctx, logger, cluster)
}
