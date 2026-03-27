package bluegreen

import (
	"context"
	"errors"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
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

	if err := upgrade.EnsureUpgradeServiceAccount(ctx, m.client, cluster, "openbao-operator"); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	m.ensureBlueGreenStatus(ctx, logger, cluster)

	initialPhase := openbaov1alpha1.PhaseIdle
	initialRollbackSet := false
	if cluster.Status.BlueGreen != nil {
		initialPhase = cluster.Status.BlueGreen.Phase
		initialRollbackSet = cluster.Status.BlueGreen.RollbackStartTime != nil
	}

	defer m.finalizeBlueGreenMetrics(metrics, strategy, cluster, initialPhase, initialRollbackSet)

	if m.shouldHaltForBreakGlass(logger, cluster) {
		return requeueStandard(), nil
	}

	if upgrade.PromoteRequestPending(cluster) &&
		(cluster.Status.BlueGreen == nil ||
			cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseSyncing ||
			!cluster.Status.BlueGreen.ManualPromotionRequired) {
		promoteRequest := upgrade.PromoteRequestValue(cluster)
		upgrade.MarkPromoteRequestHandled(&cluster.Status, promoteRequest)
		logger.Info("Ignoring promote request because no held blue/green upgrade is waiting for approval",
			"promoteRequest", promoteRequest,
			"promoteRequestField", upgrade.RequestPromoteFieldPath)
	}

	upgradeActive := cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle
	upgradeNeeded := cluster.Status.CurrentVersion != "" && cluster.Spec.Version != cluster.Status.CurrentVersion

	if handled, res, err := m.maybeAcquireUpgradeLock(ctx, logger, cluster, upgradeActive, upgradeNeeded); handled || err != nil {
		return res, err
	}

	if handled, res, err := m.handleNoUpgradeNeeded(ctx, logger, cluster); handled || err != nil {
		return res, err
	}

	if handled, res, err := m.maybeHandleTargetRevisionDrift(ctx, logger, cluster); handled || err != nil {
		return res, err
	}

	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
		if err := upgrade.ValidateUpgradeTargetVersion(logger, cluster.Status.CurrentVersion, cluster.Spec.Version); err != nil {
			return recon.Result{}, m.releaseUpgradeLockOnIdleValidationError(ctx, logger, cluster, err)
		}
		if err := upgrade.ValidateImageRefMatchesVersion(cluster.Spec.Version, cluster.Spec.Image); err != nil {
			return recon.Result{}, m.releaseUpgradeLockOnIdleValidationError(ctx, logger, cluster, err)
		}
	}

	logger.Info("Upgrade detected; CurrentVersion differs from Spec.Version",
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version)

	if handled, res, err := m.maybeAbortUpgrade(ctx, logger, cluster); handled || err != nil {
		return res, err
	}

	result, err = m.executeStateMachine(ctx, logger, cluster, verifiedImageDigest)
	return result, err
}

func (m *Manager) shouldReconcileBlueGreen(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen {
		updateStrategy := "nil"
		if cluster.Spec.Upgrade != nil {
			updateStrategy = string(cluster.Spec.Upgrade.Strategy)
		}
		logger.V(1).Info("UpdateStrategy is not BlueGreen; skipping blue/green upgrade reconciliation",
			"updateStrategy", updateStrategy)
		return false
	}
	return true
}

func (m *Manager) ensureBlueGreenStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	if m.infraRuntime == nil {
		return
	}
	m.infraRuntime.EnsureBlueGreenStatus(ctx, logger, cluster)
}

func (m *Manager) maybeAcquireUpgradeLock(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, upgradeActive, upgradeNeeded bool) (bool, recon.Result, error) {
	if !upgradeActive && !upgradeNeeded {
		return false, recon.Result{}, nil
	}
	lockHeldByUs := upgrade.IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock)
	if err := upgrade.AcquireUpgradeOperationLock(ctx, m.client, cluster, fmt.Sprintf("blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase)); err != nil {
		if upgrade.IsOperationLockHeld(err) {
			fields := map[string]string{
				"cluster_namespace": cluster.Namespace,
				"cluster_name":      cluster.Name,
				"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
				"holder":            upgrade.UpgradeOperationLockHolder,
			}
			opslifecycle.AddHeldAuditFields(fields, err)
			logging.LogAuditEvent(logger, logging.EventOperationLockBlocked, fields)
			m.emitWarningEvent(cluster, upgrade.ReasonOperationLockBlocked, "Blue/green upgrade blocked by operation lock: %v", err)
			if upgradeActive {
				return true, recon.Result{}, fmt.Errorf("blue/green upgrade in progress but operation lock is held by another operation: %w", err)
			}
			logger.Info("Blue/green upgrade blocked by operation lock", "error", err.Error())
			return true, recon.Result{RequeueAfter: opslifecycle.RequeueDelay(opslifecycle.RetryClassLockContention)}, nil
		}
		return true, recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
	}
	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
			"holder":            upgrade.UpgradeOperationLockHolder,
		})
	}
	return false, recon.Result{}, nil
}

func (m *Manager) handleNoUpgradeNeeded(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, recon.Result, error) {
	if cluster.Status.CurrentVersion == "" {
		logger.Info("CurrentVersion not yet set; waiting for initial version to be established")
		if err := m.ensureIdleAndCleanupGreen(ctx, logger, cluster); err != nil {
			return true, recon.Result{}, err
		}
		return true, requeueStandard(), nil
	}

	if cluster.Status.CurrentVersion == cluster.Spec.Version {
		logger.V(1).Info("No upgrade needed; CurrentVersion matches Spec.Version",
			"currentVersion", cluster.Status.CurrentVersion,
			"specVersion", cluster.Spec.Version)
		if err := m.ensureIdleAndCleanupGreen(ctx, logger, cluster); err != nil {
			return true, recon.Result{}, err
		}
		return true, recon.Result{}, nil
	}

	return false, recon.Result{}, nil
}

func (m *Manager) ensureIdleAndCleanupGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.BlueGreen == nil {
		return nil
	}

	shouldCleanupGreen := cluster.Status.BlueGreen.GreenRevision != ""
	if shouldCleanupGreen {
		if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
			return fmt.Errorf("failed to cleanup Green StatefulSet: %w", err)
		}
	}

	resetBlueGreenTransientState(cluster.Status.BlueGreen)

	if err := m.releaseUpgradeLockIfHeld(ctx, logger, cluster); err != nil {
		return err
	}
	return nil
}

func (m *Manager) releaseUpgradeLockOnIdleValidationError(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, cause error) error {
	if cause == nil || cluster == nil {
		return cause
	}
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		return cause
	}
	if err := m.releaseUpgradeLockIfHeld(ctx, logger, cluster); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (m *Manager) releaseUpgradeLockIfHeld(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if !upgrade.IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock) {
		return nil
	}
	if err := upgrade.ReleaseUpgradeOperationLock(ctx, m.client, cluster); err != nil {
		if upgrade.IsOperationLockHeld(err) {
			logger.V(1).Info("Upgrade operation lock changed ownership before release")
			return nil
		}
		return fmt.Errorf("failed to release upgrade operation lock: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
		"holder":            upgrade.UpgradeOperationLockHolder,
	})
	return nil
}

func (m *Manager) finalizeUpgradeTerminalState(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	promoteGreenToBlue bool,
) error {
	if cluster.Status.BlueGreen == nil {
		return nil
	}

	if promoteGreenToBlue {
		cluster.Status.BlueGreen.BlueRevision = cluster.Status.BlueGreen.GreenRevision
		if cluster.Spec.Image != "" {
			cluster.Status.BlueGreen.BlueImage = cluster.Spec.Image
		}
	}

	resetBlueGreenTransientState(cluster.Status.BlueGreen)

	return m.releaseUpgradeLockIfHeld(ctx, logger, cluster)
}

func resetBlueGreenTransientState(status *openbaov1alpha1.BlueGreenStatus) {
	if status == nil {
		return
	}
	status.Phase = openbaov1alpha1.PhaseIdle
	status.GreenRevision = ""
	status.ManualPromotionRequired = false
	status.StartTime = nil
	status.JobFailureCount = 0
	status.LastJobFailure = ""
}

// maybeHandleTargetRevisionDrift unwinds an in-flight blue/green upgrade when
// the desired Green revision changes mid-upgrade. This prevents the operator
// from silently continuing an outdated target after spec.version/image/replicas
// were changed by the user.
func (m *Manager) maybeHandleTargetRevisionDrift(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, recon.Result, error) {
	if cluster.Status.BlueGreen == nil {
		return false, recon.Result{}, nil
	}

	switch cluster.Status.BlueGreen.Phase {
	case openbaov1alpha1.PhaseIdle, openbaov1alpha1.PhaseRollingBack, openbaov1alpha1.PhaseRollbackCleanup:
		return false, recon.Result{}, nil
	}

	if cluster.Status.BlueGreen.GreenRevision == "" {
		return false, recon.Result{}, nil
	}

	desiredGreenRevision := m.calculateRevision(cluster)
	if cluster.Status.BlueGreen.GreenRevision == desiredGreenRevision {
		return false, recon.Result{}, nil
	}

	logger.Info("Spec drift detected during blue/green upgrade; unwinding current target before re-evaluating",
		"phase", cluster.Status.BlueGreen.Phase,
		"activeGreenRevision", cluster.Status.BlueGreen.GreenRevision,
		"desiredGreenRevision", desiredGreenRevision,
		"currentVersion", cluster.Status.CurrentVersion,
		"targetVersion", cluster.Spec.Version)

	result, err := m.triggerRollbackOrAbort(ctx, logger, cluster, upgrade.ReasonVersionMismatch)
	if err != nil {
		return true, recon.Result{}, err
	}
	if result == (recon.Result{}) {
		return true, requeueShort(), nil
	}
	return true, result, nil
}

func (m *Manager) maybeAbortUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, recon.Result, error) {
	shouldAbort, err := m.checkAbortConditions(ctx, logger, cluster)
	if err != nil {
		return true, recon.Result{}, fmt.Errorf("failed to check abort conditions: %w", err)
	}
	if !shouldAbort {
		return false, recon.Result{}, nil
	}
	if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
		return true, recon.Result{}, fmt.Errorf("failed to abort upgrade: %w", err)
	}
	return true, requeueShort(), nil
}
