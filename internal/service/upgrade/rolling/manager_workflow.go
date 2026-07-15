package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func (m *Manager) shouldSkipUpgradeReconcile(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, bool) {
	if !cluster.Status.Initialized {
		logger.V(1).Info("Cluster not initialized; skipping upgrade reconciliation")
		return recon.Result{RequeueAfter: constants.RequeueStandard}, true
	}

	if upgrade.EffectiveStrategy(cluster) == openbaov1alpha1.UpdateStrategyBlueGreen {
		logger.V(1).Info("Skipping rolling upgrade reconciliation; BlueGreen strategy active")
		return recon.Result{}, true
	}

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active && cluster.Spec.BreakGlassAck != cluster.Status.BreakGlass.Nonce {
		logger.Info("Cluster is in break glass mode; halting upgrade reconciliation",
			"breakGlassReason", cluster.Status.BreakGlass.Reason,
			"breakGlassNonce", cluster.Status.BreakGlass.Nonce)
		return recon.Result{RequeueAfter: constants.RequeueStandard}, true
	}

	return recon.Result{}, false
}

func (m *Manager) ensureUpgradeLock(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
	upgradeNeeded bool,
	resumeUpgrade bool,
) (recon.Result, bool, error) {
	if !upgradeNeeded && !resumeUpgrade {
		m.releaseIdleUpgradeLock(ctx, logger, cluster)
		upgrade.SetInactiveProgressMetrics(metrics)
		return recon.Result{}, true, nil
	}

	result, err := m.acquireUpgradeLock(ctx, logger, cluster, metrics, strategy)
	if err != nil || result.RequeueAfter > 0 {
		return result, true, err
	}

	return recon.Result{}, false, nil
}

func (m *Manager) releaseIdleUpgradeLock(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	if err := core.ReleaseUpgradeLockIfHeldWithReader(ctx, m.reader, m.client, logger, cluster); err != nil {
		logger.Error(err, "Failed to release stale upgrade operation lock")
	}
}

func (m *Manager) acquireUpgradeLock(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
) (recon.Result, error) {
	lockMessage := fmt.Sprintf("upgrade to %s", cluster.Spec.Version)
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.TargetVersion != "" {
		lockMessage = fmt.Sprintf("upgrade to %s (in progress)", cluster.Status.Upgrade.TargetVersion)
	}

	lockResult, err := core.AcquireUpgradeLockWithReader(ctx, m.reader, m.client, logger, cluster, lockMessage)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
	}
	if lockResult.Blocked {
		return m.handleBlockedUpgradeLock(ctx, logger, cluster, metrics, strategy, lockResult.LockErr)
	}

	return recon.Result{}, nil
}

func (m *Manager) handleBlockedUpgradeLock(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
	lockErr error,
) (recon.Result, error) {
	m.emitWarningEvent(cluster, upgrade.ReasonOperationLockBlocked, "Upgrade blocked by operation lock: %v", lockErr)

	if cluster.Status.Upgrade == nil {
		logger.Info("Upgrade blocked by operation lock", "error", lockErr.Error())
		return recon.Result{RequeueAfter: opslifecycle.RequeueDelay(opslifecycle.RetryClassLockContention)}, nil
	}

	m.recordUpgradeFailure(
		ctx,
		logger,
		cluster,
		metrics,
		strategy,
		upgrade.ReasonUpgradeFailed,
		"upgrade halted due to concurrent operation lock",
		"Failed to persist rolling upgrade status after lock contention",
	)

	return recon.Result{}, fmt.Errorf("upgrade in progress but operation lock is held by another operation: %w", lockErr)
}
