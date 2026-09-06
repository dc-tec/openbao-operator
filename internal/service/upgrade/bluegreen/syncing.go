package bluegreen

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) ensureMinimumSyncDuration(
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	if cluster.Spec.Upgrade.BlueGreen == nil ||
		cluster.Spec.Upgrade.BlueGreen.Verification == nil ||
		cluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration == "" {
		return phaseOutcome{}, false, nil
	}
	if cluster.Status.BlueGreen.StartTime == nil {
		return phaseOutcome{}, true, fmt.Errorf("StartTime is nil in Syncing phase")
	}

	minDuration, err := time.ParseDuration(cluster.Spec.Upgrade.BlueGreen.Verification.MinSyncDuration)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("invalid MinSyncDuration: %w", err)
	}

	elapsed := time.Since(cluster.Status.BlueGreen.StartTime.Time)
	if elapsed >= minDuration {
		return phaseOutcome{}, false, nil
	}

	logger.Info("Waiting for MinSyncDuration", "elapsed", elapsed, "minDuration", minDuration)
	return requeueAfterOutcome(minDuration - elapsed), true, nil
}

func (m *Manager) ensurePrePromotionHookComplete(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, true, fmt.Errorf("blue/green status is required")
	}

	var hook *openbaov1alpha1.ValidationHookConfig
	if cluster.Spec.Upgrade.BlueGreen != nil && cluster.Spec.Upgrade.BlueGreen.Verification != nil {
		hook = cluster.Spec.Upgrade.BlueGreen.Verification.PrePromotionHook
	}
	if hook == nil && cluster.Status.BlueGreen.ValidationHook == nil {
		return phaseOutcome{}, false, nil
	}

	hookResult, receiptAdvanced, err := m.reconcilePrePromotionHookJob(ctx, logger, cluster, hook)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("failed to reconcile pre-promotion hook Job: %w", err)
	}
	if receiptAdvanced || hookResult == nil {
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	hookDecision, err := prePromotionHookDecision(autoRollbackSettings(cluster), hookResult, "pre-promotion hook failed")
	if err != nil {
		return phaseOutcome{}, true, err
	}
	if !hookDecision.Handled {
		logger.Info("Pre-promotion hook completed successfully", "job", hookResult.Name)
		return phaseOutcome{}, false, nil
	}

	if hookResult.Running {
		logger.Info("Pre-promotion hook job is in progress", "job", hookResult.Name)
	}
	if hookResult.Failed {
		logger.Info("Pre-promotion hook job failed", "job", hookResult.Name)
	}
	return hookDecision.Outcome, true, nil
}

func (m *Manager) decideSyncPromotion(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if !cluster.Status.BlueGreen.ManualPromotionRequired {
		m.emitNormalEvent(cluster, ReasonBlueGreenPromotionApproved, "Promotion approved for Green revision %s", cluster.Status.BlueGreen.GreenRevision)
		return advance(openbaov1alpha1.PhasePromoting), nil
	}

	if upgrade.PromoteRequestPending(cluster) {
		promoteRequest := upgrade.PromoteRequestValue(cluster)
		logger.Info("Promotion request accepted for held blue/green upgrade",
			"promoteRequest", promoteRequest,
			"promoteRequestField", upgrade.RequestPromoteFieldPath)
		m.emitNormalEvent(cluster, ReasonBlueGreenPromotionApproved, "Promotion approved for Green revision %s", cluster.Status.BlueGreen.GreenRevision)
		outcome := advance(openbaov1alpha1.PhasePromoting)
		outcome.acknowledgements.Promote = promoteRequest
		return outcome, nil
	}

	logger.Info("Blue/green upgrade is waiting for manual approval",
		"promoteRequestField", upgrade.RequestPromoteFieldPath)
	m.emitNormalEvent(cluster, ReasonBlueGreenHoldEntered, "Blue/green upgrade is waiting for promotion approval for target version %s", cluster.Spec.Version)
	return hold(), nil
}
