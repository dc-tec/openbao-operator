package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) shouldSkipUpgradeReconcile(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, bool) {
	if !cluster.Status.Initialized {
		logger.V(1).Info("Cluster not initialized; skipping upgrade reconciliation")
		return recon.Result{RequeueAfter: constants.RequeueStandard}, true
	}

	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
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
		metrics.SetInProgress(false)
		return recon.Result{}, true, nil
	}

	result, err := m.acquireUpgradeLock(ctx, logger, cluster, metrics, strategy)
	if err != nil || result.RequeueAfter > 0 {
		return result, true, err
	}

	return recon.Result{}, false, nil
}

func (m *Manager) releaseIdleUpgradeLock(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	if !upgrade.IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock) {
		return
	}

	if err := upgrade.ReleaseUpgradeOperationLock(ctx, m.client, cluster); err != nil {
		if !upgrade.IsOperationLockHeld(err) {
			logger.Error(err, "Failed to release stale upgrade operation lock")
		}
		return
	}

	logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
		"holder":            upgrade.UpgradeOperationLockHolder,
	})
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

	lockHeldByUs := upgrade.IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock)
	if err := upgrade.AcquireUpgradeOperationLock(ctx, m.client, cluster, lockMessage); err != nil {
		if !upgrade.IsOperationLockHeld(err) {
			return recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
		}

		return m.handleBlockedUpgradeLock(logger, cluster, metrics, strategy, err)
	}

	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
			"holder":            upgrade.UpgradeOperationLockHolder,
		})
	}

	return recon.Result{}, nil
}

func (m *Manager) handleBlockedUpgradeLock(
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
	lockErr error,
) (recon.Result, error) {
	fields := map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
		"holder":            upgrade.UpgradeOperationLockHolder,
	}
	opslifecycle.AddHeldAuditFields(fields, lockErr)
	logging.LogAuditEvent(logger, logging.EventOperationLockBlocked, fields)
	m.emitWarningEvent(cluster, upgrade.ReasonOperationLockBlocked, "Upgrade blocked by operation lock: %v", lockErr)

	if cluster.Status.Upgrade == nil {
		logger.Info("Upgrade blocked by operation lock", "error", lockErr.Error())
		return recon.Result{RequeueAfter: opslifecycle.RequeueDelay(opslifecycle.RetryClassLockContention)}, nil
	}

	firstFailure := cluster.Status.Upgrade.LastErrorAt == nil
	upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonUpgradeFailed, "upgrade halted due to concurrent operation lock")
	metrics.SetStatus(upgrade.UpgradeStatusFailed)
	if firstFailure {
		metrics.IncrementFailure(strategy)
		logging.LogAuditEvent(logger, logging.EventUpgradeFailed, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"strategy":          strategy,
			"reason":            upgrade.ReasonUpgradeFailed,
		})
		m.emitWarningEvent(cluster, upgrade.ReasonUpgradeFailed, upgrade.MessageUpgradeFailed, "upgrade halted due to concurrent operation lock")
	}

	return recon.Result{}, fmt.Errorf("upgrade in progress but operation lock is held by another operation: %w", lockErr)
}

func (m *Manager) prepareUpgradeExecution(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
	resumeUpgrade bool,
) (recon.Result, bool, error) {
	if err := m.resumeUpgradeState(ctx, logger, cluster, resumeUpgrade); err != nil {
		return recon.Result{}, false, err
	}

	if err := upgrade.EnsureUpgradeServiceAccount(ctx, m.client, cluster, "openbao-operator"); err != nil {
		return recon.Result{}, false, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	if err := m.validateUpgrade(ctx, logger, cluster); err != nil {
		if persistErr := m.persistValidationFailure(ctx, cluster, err); persistErr != nil {
			return recon.Result{}, false, persistErr
		}
		return recon.Result{}, false, m.releaseUpgradeLockOnPreStartError(ctx, logger, cluster, err)
	}

	if result, waiting, err := m.ensurePreUpgradeSnapshotComplete(ctx, logger, cluster); waiting || err != nil {
		return result, waiting, err
	}

	if cluster.Status.Upgrade == nil {
		if err := m.initializeUpgrade(ctx, logger, cluster, metrics, strategy); err != nil {
			return recon.Result{}, false, err
		}
	}

	m.recordInProgressMetrics(metrics, cluster)
	return recon.Result{}, false, nil
}

func (m *Manager) resumeUpgradeState(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, resumeUpgrade bool) error {
	if !resumeUpgrade || cluster.Status.Upgrade == nil {
		return nil
	}

	if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
		logger.Info("Spec.Version changed during upgrade; clearing upgrade state and starting fresh",
			"previousTarget", cluster.Status.Upgrade.TargetVersion,
			"newTarget", cluster.Spec.Version)
		upgrade.ClearUpgrade(&cluster.Status)
	}

	_, err := m.prepareFailedUpgradeRetry(ctx, logger, cluster)
	return err
}

func (m *Manager) persistValidationFailure(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, validationErr error) error {
	if cluster.Status.Upgrade == nil {
		return nil
	}
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to persist rolling upgrade status after validation failure: %w (validation error: %w)", err, validationErr)
	}
	return nil
}

func (m *Manager) ensurePreUpgradeSnapshotComplete(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (recon.Result, bool, error) {
	if cluster.Spec.Upgrade == nil || !cluster.Spec.Upgrade.PreUpgradeSnapshot {
		return recon.Result{}, false, nil
	}

	snapshotComplete, err := m.handlePreUpgradeSnapshot(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, false, err
	}
	if snapshotComplete {
		return recon.Result{}, false, nil
	}

	logger.Info("Pre-upgrade snapshot in progress, waiting...")
	return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
}

func (m *Manager) recordInProgressMetrics(metrics *upgrade.Metrics, cluster *openbaov1alpha1.OpenBaoCluster) {
	metrics.SetInProgress(true)
	metrics.SetStatus(upgrade.UpgradeStatusRunning)
	if cluster.Status.Upgrade == nil {
		return
	}

	metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
	metrics.SetTotalPods(int(cluster.Spec.Replicas))
	metrics.SetPartition(cluster.Status.Upgrade.CurrentPartition)
}

func (m *Manager) reconcileUpgradeExecution(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
) (recon.Result, error) {
	alreadyFailed := cluster.Status.Upgrade != nil && cluster.Status.Upgrade.LastErrorAt != nil
	completed, err := m.performPodByPodUpgrade(ctx, logger, cluster, metrics)
	if err != nil {
		return m.handleUpgradeExecutionFailure(ctx, logger, cluster, metrics, strategy, alreadyFailed, err)
	}

	if !completed {
		return m.requeueAfterProgressPatch(ctx, cluster, "failed to update upgrade progress")
	}

	converged, err := m.waitForFinalizationConverged(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if !converged {
		return m.requeueAfterProgressPatch(ctx, cluster, "failed to persist upgrade progress while waiting for convergence")
	}

	if err := m.finalizeUpgrade(ctx, logger, cluster, metrics, strategy); err != nil {
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
	alreadyFailed bool,
	cause error,
) (recon.Result, error) {
	firstFailure := !alreadyFailed

	if cluster.Status.Upgrade == nil || cluster.Status.Upgrade.LastErrorReason == "" {
		upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonUpgradeFailed, cause.Error())
	}
	metrics.SetStatus(upgrade.UpgradeStatusFailed)
	if firstFailure {
		failureReason := upgrade.ReasonUpgradeFailed
		if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.LastErrorReason != "" {
			failureReason = cluster.Status.Upgrade.LastErrorReason
		}

		metrics.IncrementFailure(strategy)
		logging.LogAuditEvent(logger, logging.EventUpgradeFailed, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"strategy":          strategy,
			"reason":            failureReason,
		})

		failureMessage := cause.Error()
		if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.LastErrorMessage != "" {
			failureMessage = cluster.Status.Upgrade.LastErrorMessage
		}
		m.emitWarningEvent(cluster, failureReason, upgrade.MessageUpgradeFailed, failureMessage)
	}

	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		logger.Error(err, "Failed to update status after upgrade failure")
	}
	return recon.Result{}, cause
}

func (m *Manager) requeueAfterProgressPatch(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, errorMessage string) (recon.Result, error) {
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("%s: %w", errorMessage, err)
	}
	return recon.Result{RequeueAfter: constants.RequeueShort}, nil
}
