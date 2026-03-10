package bluegreen

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portinfra "github.com/dc-tec/openbao-operator/internal/port/infra"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

var (
	// ErrBlueGreenNotConfigured indicates that blue/green upgrade strategy is not configured.
	ErrBlueGreenNotConfigured = errors.New("blue/green upgrade strategy not configured")
	// ErrRevisionCalculationFailed indicates that revision calculation failed.
	ErrRevisionCalculationFailed = errors.New("revision calculation failed")
)

// Manager manages blue/green upgrade operations for OpenBaoCluster.
type Manager struct {
	client                client.Client
	scheme                *runtime.Scheme
	infraRuntime          portinfra.BlueGreenRuntime
	backupRuntime         portbackup.PreUpgradeSnapshotRuntime
	recorder              events.EventRecorder
	clientFactory         upgrade.OpenBaoClientFactory
	clusterOps            ClusterOps
	clientConfig          portopenbao.ClientConfig
	imageVerifier         imageverify.Verifier
	operatorImageVerifier imageverify.Verifier
	Platform              string
}

// NewManager constructs a Manager.
func NewManager(
	c client.Client,
	scheme *runtime.Scheme,
	infraRuntime portinfra.BlueGreenRuntime,
	backupRuntime portbackup.PreUpgradeSnapshotRuntime,
	clientConfig portopenbao.ClientConfig,
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	var eventRecorder events.EventRecorder
	if len(recorder) > 0 {
		eventRecorder = recorder[0]
	}
	mgr := &Manager{
		client:                c,
		scheme:                scheme,
		infraRuntime:          infraRuntime,
		backupRuntime:         backupRuntime,
		recorder:              eventRecorder,
		clientFactory:         upgrade.DefaultOpenBaoClientFactory,
		clientConfig:          clientConfig,
		imageVerifier:         imageVerifier,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
	}
	mgr.clusterOps = newOpenBaoClusterOps(c, mgr.clientFactory)
	return mgr
}

func NewManagerWithClientFactory(
	c client.Client,
	scheme *runtime.Scheme,
	infraRuntime portinfra.BlueGreenRuntime,
	backupRuntime portbackup.PreUpgradeSnapshotRuntime,
	clientFactory upgrade.OpenBaoClientFactory,
	clientConfig portopenbao.ClientConfig,
	imageVerifier imageverify.Verifier,
	operatorImageVerifier imageverify.Verifier,
	platform string,
	recorder ...events.EventRecorder,
) *Manager {
	mgr := NewManager(c, scheme, infraRuntime, backupRuntime, clientConfig, imageVerifier, operatorImageVerifier, platform, recorder...)
	if clientFactory != nil {
		mgr.clientFactory = clientFactory
	}
	mgr.clusterOps = newOpenBaoClusterOps(c, mgr.clientFactory)
	return mgr
}

func requeueShort() recon.Result {
	return recon.Result{RequeueAfter: constants.RequeueShort}
}

func requeueStandard() recon.Result {
	return recon.Result{RequeueAfter: constants.RequeueStandard}
}

func requeueAfter(duration time.Duration) recon.Result {
	if duration <= 0 {
		return recon.Result{}
	}
	return recon.Result{RequeueAfter: duration}
}

// Reconcile manages the blue/green upgrade state machine.
// This implements the controller workload sub-reconciler contract.
// Returns (result, error) where result indicates whether (and when) reconciliation should be requeued.
//
// Note: Image verification for the Blue StatefulSet is handled by the infra reconciler which runs before this.
// For Green resources (StatefulSet and snapshot Jobs), we verify and pin digests here to ensure the
// target images are validated when ImageVerification is enabled.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	updateStrategy := "RollingUpdate"
	if cluster.Spec.Upgrade != nil {
		updateStrategy = string(cluster.Spec.Upgrade.Strategy)
	}
	logger.Info("Manager reconciling",
		"updateStrategy", updateStrategy,
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version,
		"initialized", cluster.Status.Initialized,
		"blueGreenPhase", func() string {
			if cluster.Status.BlueGreen == nil {
				return "nil"
			}
			return string(cluster.Status.BlueGreen.Phase)
		}())

	// Use spec image (infra reconciler handles verification)
	verifiedImageDigest := cluster.Spec.Image

	handled, result := m.handleBreakGlassAck(logger, cluster)
	if handled {
		return result, nil
	}

	if handled, result, err := m.handleManualRollbackRequest(ctx, logger, cluster); handled || err != nil {
		return result, err
	}

	return m.reconcileBlueGreen(ctx, logger, cluster, verifiedImageDigest)
}

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

func (m *Manager) handleManualRollbackRequest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, recon.Result, error) {
	if !upgrade.RollbackRequestPending(cluster) {
		return false, recon.Result{}, nil
	}

	rollbackRequest := upgrade.RollbackRequestValue(cluster)

	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
		upgrade.MarkRollbackRequestHandled(&cluster.Status, rollbackRequest)
		logger.Info("Ignoring rollback request because no blue/green upgrade is active",
			"rollbackRequest", rollbackRequest,
			"rollbackRequestField", upgrade.RequestRollbackFieldPath)
		return false, recon.Result{}, nil
	}

	if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollingBack ||
		cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollbackCleanup {
		upgrade.MarkRollbackRequestHandled(&cluster.Status, rollbackRequest)
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
		upgrade.MarkRollbackRequestHandled(&cluster.Status, rollbackRequest)
		return true, recon.Result{}, nil
	}

	result, err := m.triggerRollback(logger, cluster, fmt.Sprintf("manual rollback request via %s", upgrade.RequestRollbackFieldPath))
	if err == nil {
		upgrade.MarkRollbackRequestHandled(&cluster.Status, rollbackRequest)
	}
	return true, result, err
}

func (m *Manager) finalizeBlueGreenMetrics(metrics *upgrade.Metrics, strategy string, cluster *openbaov1alpha1.OpenBaoCluster, initialPhase openbaov1alpha1.BlueGreenPhase, initialRollbackSet bool) {
	if metrics == nil || cluster == nil {
		return
	}

	phase := openbaov1alpha1.PhaseIdle
	if cluster.Status.BlueGreen != nil {
		phase = cluster.Status.BlueGreen.Phase
	}

	inProgress := phase != openbaov1alpha1.PhaseIdle
	metrics.SetInProgress(inProgress)
	if inProgress {
		metrics.SetStatus(upgrade.UpgradeStatusRunning)
		metrics.SetTotalPods(int(cluster.Spec.Replicas))
	} else {
		// Leave status unchanged when idle so the last terminal status (success/failed)
		// can be observed after the upgrade completes.
		metrics.SetTotalPods(0)
	}
	metrics.SetPodsCompleted(0)
	metrics.SetPartition(0)

	state, ok := getUpgradeMetricsState(cluster.Namespace, cluster.Name)
	if !ok && initialPhase != openbaov1alpha1.PhaseIdle {
		startedAt := time.Now()
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.StartTime != nil {
			startedAt = cluster.Status.BlueGreen.StartTime.Time
		}
		state = upgradeMetricsState{startedAt: startedAt}
		ok = true
		setUpgradeMetricsState(cluster.Namespace, cluster.Name, state)
	}

	// If a new upgrade started this reconcile, initialize state and increment totals once.
	if initialPhase == openbaov1alpha1.PhaseIdle && phase != openbaov1alpha1.PhaseIdle {
		if _, exists := getUpgradeMetricsState(cluster.Namespace, cluster.Name); !exists {
			setUpgradeMetricsState(cluster.Namespace, cluster.Name, upgradeMetricsState{startedAt: time.Now()})
			metrics.IncrementTotal(strategy)
			state, ok = getUpgradeMetricsState(cluster.Namespace, cluster.Name)
		}
	}

	// Rollback initiation: count once when RollbackStartTime is first set.
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.RollbackStartTime != nil && !initialRollbackSet {
		if ok {
			state.lastRollbackSeen = true
			setUpgradeMetricsState(cluster.Namespace, cluster.Name, state)
		}
		metrics.IncrementRollback(strategy)
		metrics.IncrementFailure(strategy)
	}

	// Completion: a transition from any non-idle phase to idle.
	if initialPhase != openbaov1alpha1.PhaseIdle && phase == openbaov1alpha1.PhaseIdle && ok {
		durationSeconds := time.Since(state.startedAt).Seconds()
		metrics.RecordDuration(durationSeconds, cluster.Status.CurrentVersion, cluster.Spec.Version)
		deleteUpgradeMetricsState(cluster.Namespace, cluster.Name)

		if initialPhase == openbaov1alpha1.PhaseCleanup {
			metrics.IncrementSuccess(strategy)
			metrics.SetStatus(upgrade.UpgradeStatusSuccess)
			return
		}

		if !state.lastRollbackSeen {
			metrics.IncrementFailure(strategy)
		}
		metrics.SetStatus(upgrade.UpgradeStatusFailed)
	}
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
			// Use RequeueShort to check more frequently when waiting for backup/restore to complete
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
	// CRITICAL: If CurrentVersion is empty, the cluster is in initial state (not yet set by controller).
	// We should NOT trigger an upgrade until CurrentVersion is set. An upgrade is only needed when
	// CurrentVersion is set AND different from Spec.Version.
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

// calculateRevision computes a deterministic revision hash from relevant spec fields.
func (m *Manager) calculateRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
}

// transitionToPhase is a helper that sets the phase and restarts the StartTime timer.
// This reduces boilerplate in phase handlers.
// It also resets the job failure count when transitioning phases.
func (m *Manager) transitionToPhase(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, phase openbaov1alpha1.BlueGreenPhase) {
	previousPhase := cluster.Status.BlueGreen.Phase
	cluster.Status.BlueGreen.Phase = phase
	if phase == openbaov1alpha1.PhaseIdle {
		cluster.Status.BlueGreen.StartTime = nil
	} else {
		now := metav1.Now()
		cluster.Status.BlueGreen.StartTime = &now
	}
	// Reset job failure count on phase transition
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""
	opslifecycle.LogPhaseTransition(logger, logging.EventBlueGreenPhaseTransition, string(previousPhase), string(phase), map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
	})
}

// executeStateMachine runs the blue/green upgrade state machine.
func (m *Manager) executeStateMachine(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) (recon.Result, error) {
	phase := cluster.Status.BlueGreen.Phase

	logger = logger.WithValues("phase", phase)

	type phaseHandler func(context.Context, logr.Logger, *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error)

	handlers := map[openbaov1alpha1.BlueGreenPhase]phaseHandler{
		openbaov1alpha1.PhaseIdle: func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
			return m.handlePhaseIdle(ctx, logger, cluster, verifiedImageDigest)
		},
		openbaov1alpha1.PhaseDeployingGreen: func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
			return m.handlePhaseDeployingGreen(ctx, logger, cluster, verifiedImageDigest)
		},
		openbaov1alpha1.PhaseJoiningMesh:     m.handlePhaseJoiningMesh,
		openbaov1alpha1.PhaseSyncing:         m.handlePhaseSyncing,
		openbaov1alpha1.PhasePromoting:       m.handlePhasePromoting,
		openbaov1alpha1.PhaseDemotingBlue:    m.handlePhaseDemotingBlue,
		openbaov1alpha1.PhaseCleanup:         m.handlePhaseCleanup,
		openbaov1alpha1.PhaseRollingBack:     m.handlePhaseRollingBack,
		openbaov1alpha1.PhaseRollbackCleanup: m.handlePhaseRollbackCleanup,
	}

	handler, ok := handlers[phase]
	if !ok {
		return recon.Result{}, fmt.Errorf("unknown blue/green phase: %s", phase)
	}

	outcome, err := handler(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	return m.applyOutcome(ctx, logger, cluster, outcome)
}

func (m *Manager) applyOutcome(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, outcome phaseOutcome) (recon.Result, error) {
	if err := outcome.validate(); err != nil {
		return recon.Result{}, err
	}

	switch outcome.kind {
	case phaseOutcomeAdvance:
		m.transitionToPhase(logger, cluster, outcome.nextPhase)
		if outcome.nextPhase == openbaov1alpha1.PhaseIdle {
			return recon.Result{}, nil
		}
		return requeueShort(), nil
	case phaseOutcomeRequeueAfter:
		return requeueAfter(outcome.after), nil
	case phaseOutcomeHold:
		return recon.Result{}, nil
	case phaseOutcomeRollback:
		return m.triggerRollbackOrAbort(ctx, logger, cluster, outcome.reason)
	case phaseOutcomeAbort:
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return recon.Result{}, err
		}
		return recon.Result{}, nil
	case phaseOutcomeDone:
		return recon.Result{}, nil
	default:
		return recon.Result{}, fmt.Errorf("unknown outcome kind: %q", outcome.kind)
	}
}
