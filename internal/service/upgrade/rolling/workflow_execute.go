package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) reconcileUpgradeExecution(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
) (recon.Result, error) {
	completed, err := m.performPodByPodUpgrade(ctx, logger, cluster, metrics)
	if err != nil {
		return m.handleUpgradeExecutionFailure(ctx, logger, cluster, metrics, strategy, err)
	}
	if !completed {
		return m.requeueAfterProgressPatch(ctx, cluster, "failed to update upgrade progress")
	}

	return m.finalizeConvergedUpgrade(ctx, logger, cluster, metrics, strategy)
}

func (m *Manager) finalizeConvergedUpgrade(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
) (recon.Result, error) {
	converged, err := m.waitForFinalizationConverged(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if !converged {
		return m.requeueAfterProgressPatch(ctx, cluster, "failed to persist upgrade progress while waiting for convergence")
	}

	if err := upgrade.CompleteRootUpgradeLifecycle(ctx, logger, cluster, metrics, strategy, upgrade.RootUpgradeCompletionOptions{
		Persist: func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, _ upgrade.RootUpgradeSessionCompletion) error {
			if err := m.patchFinalizedUpgradeStatus(ctx, cluster); err != nil {
				return fmt.Errorf("failed to update status after completing upgrade: %w", err)
			}
			return nil
		},
		EmitEvent: func(fromVersion, toVersion string) {
			m.emitNormalEvent(cluster, upgrade.ReasonUpgradeComplete, upgrade.MessageUpgradeComplete, fromVersion, toVersion)
		},
	}); err != nil {
		return recon.Result{}, err
	}

	return recon.Result{}, nil
}

func (m *Manager) handleUpgradeExecutionFailure(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
	cause error,
) (recon.Result, error) {
	m.recordUpgradeFailure(
		ctx,
		logger,
		cluster,
		metrics,
		strategy,
		upgrade.ReasonUpgradeFailed,
		cause.Error(),
		"Failed to update status after upgrade failure",
	)
	return recon.Result{}, cause
}

func (m *Manager) requeueAfterProgressPatch(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, errorMessage string) (recon.Result, error) {
	if err := m.patchUpgradeStatus(ctx, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("%s: %w", errorMessage, err)
	}
	return recon.Result{RequeueAfter: constants.RequeueShort}, nil
}
