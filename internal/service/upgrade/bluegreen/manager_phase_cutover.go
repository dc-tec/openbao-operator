package bluegreen

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// handlePhaseSyncing waits for Green nodes to catch up with Blue nodes.
func (m *Manager) handlePhaseSyncing(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	if cluster.Spec.Upgrade.BlueGreen != nil &&
		cluster.Spec.Upgrade.BlueGreen.Verification != nil &&
		cluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration != "" {
		if cluster.Status.BlueGreen.StartTime == nil {
			return phaseOutcome{}, fmt.Errorf("StartTime is nil in Syncing phase")
		}

		minDuration, err := time.ParseDuration(cluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration)
		if err != nil {
			return phaseOutcome{}, fmt.Errorf("invalid MinSyncDuration: %w", err)
		}

		elapsed := time.Since(cluster.Status.BlueGreen.StartTime.Time)
		if elapsed < minDuration {
			logger.Info("Waiting for MinSyncDuration", "elapsed", elapsed, "minDuration", minDuration)
			return requeueAfterOutcome(minDuration - elapsed), nil
		}
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionWaitGreenSynced, "job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	if cluster.Spec.Upgrade.BlueGreen != nil &&
		cluster.Spec.Upgrade.BlueGreen.Verification != nil &&
		cluster.Spec.Upgrade.BlueGreen.Verification.PrePromotionHook != nil {
		hook := cluster.Spec.Upgrade.BlueGreen.Verification.PrePromotionHook
		hookResult, err := m.ensurePrePromotionHookJob(ctx, logger, cluster, hook)
		if err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to ensure pre-promotion hook job: %w", err)
		}
		hookDecision, err := prePromotionHookDecision(autoRollbackSettings(cluster), hookResult, "pre-promotion hook failed")
		if err != nil {
			return phaseOutcome{}, err
		}
		if hookDecision.Handled {
			if hookResult.Running {
				logger.Info("Pre-promotion hook job is in progress", "job", hookResult.Name)
			}
			if hookResult.Failed {
				logger.Info("Pre-promotion hook job failed", "job", hookResult.Name)
			}
			return hookDecision.Outcome, nil
		}
		logger.Info("Pre-promotion hook completed successfully", "job", hookResult.Name)
	}

	if cluster.Status.BlueGreen.ManualPromotionRequired {
		if upgrade.PromoteRequestPending(cluster) {
			promoteRequest := upgrade.PromoteRequestValue(cluster)
			upgrade.MarkPromoteRequestHandled(&cluster.Status, promoteRequest)
			logger.Info("Promotion request accepted for held blue/green upgrade",
				"promoteRequest", promoteRequest,
				"promoteRequestField", upgrade.RequestPromoteFieldPath)
			m.emitNormalEvent(cluster, ReasonBlueGreenPromotionApproved, "Promotion approved for Green revision %s", cluster.Status.BlueGreen.GreenRevision)
			return advance(openbaov1alpha1.PhasePromoting), nil
		}

		logger.Info("Blue/green upgrade is waiting for manual approval",
			"promoteRequestField", upgrade.RequestPromoteFieldPath)
		m.emitNormalEvent(cluster, ReasonBlueGreenHoldEntered, "Blue/green upgrade is waiting for promotion approval for target version %s", cluster.Spec.Version)
		return hold(), nil
	}

	m.emitNormalEvent(cluster, ReasonBlueGreenPromotionApproved, "Promotion approved for Green revision %s", cluster.Status.BlueGreen.GreenRevision)
	return advance(openbaov1alpha1.PhasePromoting), nil
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

	metrics := upgrade.NewMetrics(cluster.Namespace, cluster.Name)
	state, ok := getUpgradeMetricsState(cluster.Namespace, cluster.Name)
	if !ok {
		state = upgradeMetricsState{startedAt: time.Now()}
	}
	if !state.stepDownCounted {
		metrics.IncrementStepDownTotal()
		state.stepDownCounted = true
		setUpgradeMetricsState(cluster.Namespace, cluster.Name, state)
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	if greenRevision == "" {
		return phaseOutcome{}, fmt.Errorf("green revision is empty in DemotingBlue phase")
	}

	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Green pods: %w", err)
	}
	greenSnapshots, err := podSnapshotsFromPods(greenPods)
	if err != nil {
		return phaseOutcome{}, err
	}
	ok, message := demotionPreconditionsSatisfied(greenSnapshots, int(cluster.Spec.Replicas))
	if !ok {
		logger.Info(message)
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

	leaderPod, source, ok := m.clusterOps.FindLeaderPod(ctx, logger, cluster, greenPods)
	if !ok {
		logger.Info("Green leader not yet elected after demotion, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	logger.Info("Green leader confirmed after demotion", "pod", leaderPod, "source", source)
	return advance(openbaov1alpha1.PhaseCleanup), nil
}

// handlePhaseCleanup ejects Blue nodes from Raft and deletes the Blue StatefulSet.
// This is the point of no return; after this, rollback is not possible.
func (m *Manager) handlePhaseCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision
	if greenRevision == "" {
		return phaseOutcome{}, fmt.Errorf("green revision is empty in Cleanup phase")
	}

	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Green pods: %w", err)
	}
	greenSnapshots, err := podSnapshotsFromPods(greenPods)
	if err != nil {
		return phaseOutcome{}, err
	}

	leaderOK := leaderObserved(greenSnapshots)
	if !leaderOK {
		if _, source, ok := m.clusterOps.FindLeaderPod(ctx, logger, cluster, greenPods); ok {
			leaderOK = true
			logger.V(1).Info("Green leader observed via API fallback", "source", source)
		}
	}

	ok, message := cleanupPreconditionsSatisfied(greenSnapshots, int(cluster.Spec.Replicas), leaderOK)
	if !ok {
		logger.Info(message)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionRemoveBluePeers, "cleanup peer removal job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	blueStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, blueRevision)
	blueStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      blueStatefulSetName,
	}, blueStatefulSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return phaseOutcome{}, fmt.Errorf("failed to get Blue StatefulSet: %w", err)
		}
		logger.Info("Blue StatefulSet already deleted", "blueRevision", blueRevision)
	} else {
		if err := m.client.Delete(ctx, blueStatefulSet); err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to delete Blue StatefulSet: %w", err)
		}
		logger.Info("Deleted Blue StatefulSet", "blueRevision", blueRevision)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to check Blue pods: %w", err)
	}
	activeBluePods := 0
	for _, pod := range bluePods {
		if pod.DeletionTimestamp == nil {
			activeBluePods++
		}
	}
	if activeBluePods > 0 {
		logger.Info("Blue pods still exist, waiting for termination", "count", activeBluePods)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	if err := m.finalizeUpgradeTerminalState(ctx, logger, cluster, true); err != nil {
		logger.Error(err, "Failed to finalize blue/green terminal state")
		return phaseOutcome{}, err
	}

	logger.Info("Blue/green upgrade completed", "newVersion", cluster.Spec.Version)
	logging.LogAuditEvent(logger, logging.EventUpgradeCompleted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"strategy":          string(openbaov1alpha1.UpdateStrategyBlueGreen),
		"version":           cluster.Spec.Version,
	})
	m.emitNormalEvent(cluster, ReasonUpgradeComplete, "Blue/green upgrade completed for target version %s", cluster.Spec.Version)

	return requeueAfterOutcome(constants.RequeueShort), nil
}
