package bluegreen

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/config"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/infra"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/revision"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

var (
	// ErrBlueGreenNotConfigured indicates that blue/green upgrade strategy is not configured.
	ErrBlueGreenNotConfigured = errors.New("blue/green upgrade strategy not configured")
	// ErrRevisionCalculationFailed indicates that revision calculation failed.
	ErrRevisionCalculationFailed = errors.New("revision calculation failed")
)

// Manager manages blue/green upgrade operations for OpenBaoCluster.
type Manager struct {
	client        client.Client
	scheme        *runtime.Scheme
	infraManager  *infra.Manager
	clientFactory OpenBaoClientFactory
}

// OpenBaoClientFactory creates OpenBao API clients for connecting to cluster pods.
// This is primarily used for testing to inject mock clients.
type OpenBaoClientFactory func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error)

// NewManager constructs a Manager.
func NewManager(c client.Client, scheme *runtime.Scheme, infraManager *infra.Manager) *Manager {
	return &Manager{
		client:       c,
		scheme:       scheme,
		infraManager: infraManager,
		clientFactory: func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error) {
			return openbaoapi.NewClient(config)
		},
	}
}

func NewManagerWithClientFactory(c client.Client, scheme *runtime.Scheme, infraManager *infra.Manager, clientFactory OpenBaoClientFactory) *Manager {
	mgr := NewManager(c, scheme, infraManager)
	if clientFactory != nil {
		mgr.clientFactory = clientFactory
	}
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
// This implements the SubReconciler interface.
// Returns (result, error) where result indicates whether (and when) reconciliation should be requeued.
//
// Note: Image verification for the Blue StatefulSet is handled by the infra reconciler which runs before this.
// For Green resources (StatefulSet and snapshot Jobs), we verify and pin digests here to ensure the
// target images are validated when ImageVerification is enabled.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	logger.Info("Manager reconciling",
		"updateStrategy", cluster.Spec.UpdateStrategy.Type,
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version,
		"initialized", cluster.Status.Initialized,
		"blueGreenPhase", func() string {
			if cluster.Status.BlueGreen == nil {
				return "nil"
			}
			return string(cluster.Status.BlueGreen.Phase)
		}())

	// Break-glass escape hatch: allow operators to force a rollback of the
	// current blue/green upgrade via annotation. This is evaluated early so
	// that a stuck state machine can be unwound deterministically.
	if cluster.Annotations != nil {
		if value, ok := cluster.Annotations[AnnotationForceRollback]; ok && value == "true" {
			if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
				// If rollback is already in progress, do not re-trigger rollback on every reconcile.
				// Allow the normal state machine to continue creating/observing rollback Jobs.
				if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollingBack ||
					cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollbackCleanup {
					logger.Info("Force rollback annotation detected during rollback; continuing rollback state machine",
						"annotation", AnnotationForceRollback,
						"phase", cluster.Status.BlueGreen.Phase)
				} else {
					logger.Info("Force rollback annotation detected, initiating rollback",
						"annotation", AnnotationForceRollback,
						"phase", cluster.Status.BlueGreen.Phase)

					// If there is no Green revision yet, abort behaves more safely than rollback.
					if cluster.Status.BlueGreen.GreenRevision == "" {
						if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
							return recon.Result{}, fmt.Errorf("failed to abort upgrade via force-rollback annotation: %w", err)
						}
						return recon.Result{}, nil
					}

					return m.triggerRollback(logger, cluster, "manual force-rollback annotation")
				}
			}
		}
	}

	// Use spec image (infra reconciler handles verification)
	verifiedImageDigest := cluster.Spec.Image

	handled, result := m.handleBreakGlassAck(logger, cluster)
	if handled {
		return result, nil
	}

	return m.reconcileBlueGreen(ctx, logger, cluster, verifiedImageDigest)
}

// reconcileBlueGreen is the internal reconcile method that handles blue/green upgrades.
func (m *Manager) reconcileBlueGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) (recon.Result, error) {
	if !m.shouldReconcileBlueGreen(logger, cluster) {
		return recon.Result{}, nil
	}

	if !cluster.Status.Initialized {
		logger.Info("Cluster not initialized; skipping blue/green upgrade reconciliation")
		return requeueStandard(), nil
	}

	if err := upgrade.EnsureUpgradeServiceAccount(ctx, m.client, cluster, "openbao-operator"); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	m.ensureBlueGreenStatus(ctx, logger, cluster)

	if m.shouldHaltForBreakGlass(logger, cluster) {
		return requeueStandard(), nil
	}

	upgradeActive := cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle
	upgradeNeeded := cluster.Status.CurrentVersion != "" && cluster.Spec.Version != cluster.Status.CurrentVersion

	if handled, res, err := m.maybeAcquireUpgradeLock(ctx, logger, cluster, upgradeActive, upgradeNeeded); handled || err != nil {
		return res, err
	}

	if handled, res, err := m.handleNoUpgradeNeeded(ctx, logger, cluster); handled || err != nil {
		return res, err
	}

	logger.Info("Upgrade detected; CurrentVersion differs from Spec.Version",
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version)

	if handled, res, err := m.maybeAbortUpgrade(ctx, logger, cluster); handled || err != nil {
		return res, err
	}

	return m.executeStateMachine(ctx, logger, cluster, verifiedImageDigest)
}

func (m *Manager) shouldReconcileBlueGreen(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.UpdateStrategy.Type != openbaov1alpha1.UpdateStrategyBlueGreen {
		logger.V(1).Info("UpdateStrategy is not BlueGreen; skipping blue/green upgrade reconciliation",
			"updateStrategy", cluster.Spec.UpdateStrategy.Type)
		return false
	}
	return true
}

func (m *Manager) ensureBlueGreenStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster.Status.BlueGreen == nil {
		inferred, err := infra.InferActiveRevisionFromPods(ctx, m.client, cluster)
		if err != nil {
			logger.Error(err, "Failed to infer active revision from pods; falling back to spec-derived revision")
		}
		blueRevision := inferred
		if blueRevision == "" {
			blueRevision = m.calculateRevision(cluster)
		}
		cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase:        openbaov1alpha1.PhaseIdle,
			BlueRevision: blueRevision,
		}
		return
	}

	// If the operator restarted and BlueRevision was derived from spec (target) rather than the
	// currently running ("Blue") pods, correct it before starting a new upgrade or snapshot.
	if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle &&
		(cluster.Status.BlueGreen.BlueRevision == "" || cluster.Status.CurrentVersion != cluster.Spec.Version) {
		inferred, err := infra.InferActiveRevisionFromPods(ctx, m.client, cluster)
		if err != nil {
			logger.Error(err, "Failed to infer active revision from pods; keeping existing BlueRevision", "blueRevision", cluster.Status.BlueGreen.BlueRevision)
			return
		}
		if inferred != "" && inferred != cluster.Status.BlueGreen.BlueRevision {
			logger.Info("Correcting BlueRevision from active pods", "from", cluster.Status.BlueGreen.BlueRevision, "to", inferred)
			cluster.Status.BlueGreen.BlueRevision = inferred
		}
	}
}

func (m *Manager) maybeAcquireUpgradeLock(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, upgradeActive, upgradeNeeded bool) (bool, recon.Result, error) {
	if !upgradeActive && !upgradeNeeded {
		return false, recon.Result{}, nil
	}
	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    constants.ControllerNameOpenBaoCluster + "/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   fmt.Sprintf("blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase),
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			if upgradeActive {
				return true, recon.Result{}, fmt.Errorf("blue/green upgrade in progress but operation lock is held by another operation: %w", err)
			}
			logger.Info("Blue/green upgrade blocked by operation lock", "error", err.Error())
			return true, requeueStandard(), nil
		}
		return true, recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
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
	if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
		return nil
	}
	if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
		return fmt.Errorf("failed to cleanup Green StatefulSet: %w", err)
	}
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
	cluster.Status.BlueGreen.GreenRevision = ""
	cluster.Status.BlueGreen.StartTime = nil
	return nil
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
func (m *Manager) transitionToPhase(cluster *openbaov1alpha1.OpenBaoCluster, phase openbaov1alpha1.BlueGreenPhase) {
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
		openbaov1alpha1.PhaseJoiningMesh:      m.handlePhaseJoiningMesh,
		openbaov1alpha1.PhaseSyncing:          m.handlePhaseSyncing,
		openbaov1alpha1.PhasePromoting:        m.handlePhasePromoting,
		openbaov1alpha1.PhaseTrafficSwitching: m.handlePhaseTrafficSwitching,
		openbaov1alpha1.PhaseDemotingBlue:     m.handlePhaseDemotingBlue,
		openbaov1alpha1.PhaseCleanup:          m.handlePhaseCleanup,
		openbaov1alpha1.PhaseRollingBack:      m.handlePhaseRollingBack,
		openbaov1alpha1.PhaseRollbackCleanup:  m.handlePhaseRollbackCleanup,
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
		m.transitionToPhase(cluster, outcome.nextPhase)
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

// handlePhaseIdle transitions from Idle to DeployingGreen when an upgrade is detected.
func (m *Manager) handlePhaseIdle(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	logger.Info("Starting blue/green upgrade",
		"fromVersion", cluster.Status.CurrentVersion,
		"targetVersion", cluster.Spec.Version)

	// Pre-upgrade snapshot (if enabled)
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.PreUpgradeSnapshot {
		jobName := preUpgradeSnapshotJobName(cluster)
		if cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != jobName {
			_, err := m.ensurePreUpgradeSnapshotJob(ctx, logger, cluster, jobName)
			if err != nil {
				logger.Error(err, "Failed to ensure pre-upgrade snapshot job")
				return phaseOutcome{}, err // Block upgrade on snapshot failure
			}
			cluster.Status.BlueGreen.PreUpgradeSnapshotJobName = jobName
			logger.Info("Pre-upgrade snapshot job created", "job", jobName)
			return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to wait for snapshot
		} else {
			jobStatus, err := getJobStatus(ctx, m.client, cluster, jobName)
			if err != nil {
				logger.Error(err, "Failed to check pre-upgrade snapshot job status")
				// Job error - continue with upgrade
			} else if jobStatus.Exists && jobStatus.Running {
				logger.Info("Waiting for pre-upgrade snapshot to complete", "job", jobName)
				return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to wait
			}
			logger.Info("Pre-upgrade snapshot completed",
				"job", jobName)
		}
	}

	// Calculate Green revision
	greenRevision := m.calculateRevision(cluster)
	cluster.Status.BlueGreen.GreenRevision = greenRevision

	return advance(openbaov1alpha1.PhaseDeployingGreen), nil
}

// handlePhaseDeployingGreen creates the Green StatefulSet.
// IMPORTANT: Green pods must join the existing Blue cluster as non-voters, not initialize a new cluster.
func (m *Manager) handlePhaseDeployingGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision
	logger = logger.WithValues("greenRevision", greenRevision, "blueRevision", blueRevision)

	// CRITICAL: Before creating Green StatefulSet, ensure Blue pods are ready and available.
	// Green pods must join an existing initialized cluster, not form a new one.
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Blue pods: %w", err)
	}

	if len(bluePods) == 0 {
		logger.Info("No Blue pods found yet, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	// Verify Blue pods have the revision label (required for retry_join to work).
	bluePodsHaveRevisionLabel := true
	for _, pod := range bluePods {
		rev, present := pod.Labels[constants.LabelOpenBaoRevision]
		if !present || rev != blueRevision {
			bluePodsHaveRevisionLabel = false
			logger.Info("Blue pod missing revision label, InfraManager will update it",
				"pod", pod.Name,
				"expectedRevision", blueRevision,
				"actualRevision", rev)
			break
		}
	}

	if !bluePodsHaveRevisionLabel {
		logger.Info("Blue pods missing revision label; waiting for InfraManager to update StatefulSet")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	// Verify at least one Blue pod is ready and unsealed.
	blueReady := false
	for _, pod := range bluePods {
		if isPodReady(&pod) {
			sealed, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
			if err == nil && present && !sealed {
				blueReady = true
				break
			}
		}
	}

	if !blueReady {
		logger.Info("Blue pods not ready/unsealed yet, waiting before creating Green StatefulSet")
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	greenStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, greenRevision)
	greenStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      greenStatefulSetName,
	}, greenStatefulSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return phaseOutcome{}, fmt.Errorf("failed to get Green StatefulSet: %w", err)
		}

		infraDetails := configbuilder.InfrastructureDetails{
			HeadlessServiceName:   cluster.Name,
			Namespace:             cluster.Namespace,
			APIPort:               constants.PortAPI,
			ClusterPort:           constants.PortCluster,
			TargetRevisionForJoin: blueRevision,
		}

		renderedConfig, err := configbuilder.RenderHCL(cluster, infraDetails)
		if err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to render config for Green cluster: %w", err)
		}
		configContent := string(renderedConfig)

		greenImage := cluster.Spec.Image
		verifiedGreenDigest, err := m.verifyImageDigest(ctx, logger, cluster, greenImage, constants.ReasonBlueGreenImageVerificationFailed, "Green image verification failed")
		if err != nil {
			return phaseOutcome{}, err
		}

		initImage := initContainerImage(cluster)
		verifiedInitContainerDigest, err := m.verifyOperatorImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Green init container image verification failed")
		if err != nil {
			return phaseOutcome{}, err
		}

		imageForGreen := greenImage
		if verifiedGreenDigest != "" {
			imageForGreen = verifiedGreenDigest
		}

		if err := m.infraManager.EnsureStatefulSetWithRevision(ctx, logger, cluster, configContent, imageForGreen, verifiedInitContainerDigest, greenRevision, true); err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to create Green StatefulSet: %w", err)
		}

		logger.Info("Created Green StatefulSet", "greenRevision", greenRevision)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	desiredReplicas := cluster.Spec.Replicas
	if greenStatefulSet.Spec.Replicas != nil {
		desiredReplicas = *greenStatefulSet.Spec.Replicas
	}

	if greenStatefulSet.Status.ReadyReplicas < desiredReplicas {
		logger.Info("Waiting for Green pods to be ready",
			"readyReplicas", greenStatefulSet.Status.ReadyReplicas,
			"desiredReplicas", desiredReplicas)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Green pods: %w", err)
	}

	for _, pod := range greenPods {
		if pod.Status.Phase != corev1.PodRunning {
			logger.Info("Green pod not yet running", "pod", pod.Name, "phase", pod.Status.Phase)
			return requeueAfterOutcome(constants.RequeueShort), nil
		}
	}

	for _, pod := range greenPods {
		sealed, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
		if err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to parse sealed label on pod %s: %w", pod.Name, err)
		}
		if !present || sealed {
			logger.Info("Waiting for Green pod to be unsealed", "pod", pod.Name)
			return requeueAfterOutcome(constants.RequeueShort), nil
		}
	}

	return advance(openbaov1alpha1.PhaseJoiningMesh), nil
}

// handlePhaseJoiningMesh joins Green pods to the Raft cluster as non-voters.
func (m *Manager) handlePhaseJoiningMesh(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionJoinGreenNonVoters, "job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	// All pods joined, transition to Syncing
	return advance(openbaov1alpha1.PhaseSyncing), nil
}

// handlePhaseSyncing waits for Green nodes to catch up with Blue nodes.
func (m *Manager) handlePhaseSyncing(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	// Check MinSyncDuration if configured
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification.MinSyncDuration != "" {
		if cluster.Status.BlueGreen.StartTime == nil {
			return phaseOutcome{}, fmt.Errorf("StartTime is nil in Syncing phase")
		}

		minDuration, err := time.ParseDuration(cluster.Spec.UpdateStrategy.BlueGreen.Verification.MinSyncDuration)
		if err != nil {
			return phaseOutcome{}, fmt.Errorf("invalid MinSyncDuration: %w", err)
		}

		elapsed := time.Since(cluster.Status.BlueGreen.StartTime.Time)
		if elapsed < minDuration {
			logger.Info("Waiting for MinSyncDuration",
				"elapsed", elapsed,
				"minDuration", minDuration)
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

	// Check for pre-promotion hook
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification.PrePromotionHook != nil {

		hook := cluster.Spec.UpdateStrategy.BlueGreen.Verification.PrePromotionHook
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

	// Check if AutoPromote is disabled
	if cluster.Spec.UpdateStrategy.BlueGreen != nil && !cluster.Spec.UpdateStrategy.BlueGreen.AutoPromote {
		logger.Info("AutoPromote is disabled; waiting for manual approval")
		// Stay in Syncing phase until manual approval (annotation or field update)
		return hold(), nil
	}

	// All nodes synced, transition to Promoting
	return advance(openbaov1alpha1.PhasePromoting), nil
}

// handlePhasePromoting promotes Green nodes to voters.
// In OpenBao's Raft, non-voters automatically become voters when they catch up,
// but we verify this and ensure all Green nodes are voters before proceeding.
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

	// Transition to TrafficSwitching (instead of DemotingBlue)
	// This decouples traffic switching from Blue demotion for safer rollbacks
	return advance(openbaov1alpha1.PhaseTrafficSwitching), nil
}

// handlePhaseDemotingBlue demotes Blue nodes to non-voters and verifies Green becomes leader.
// After demotion, Blue nodes are no longer voters, so Green nodes (the only voters) will win any election.
// This phase includes the former "Cutover" logic - verifying Green is leader before proceeding.
func (m *Manager) handlePhaseDemotingBlue(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision

	// Safety gate: ensure Green cluster is healthy enough to take over before
	// attempting to demote Blue voters. This prevents entering a Raft
	// configuration where Green cannot form quorum after demotion.
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

	// If using GatewayWeights strategy, only proceed once we have reached the final
	// traffic step (0% Blue / 100% Green).
	strategy := openbaov1alpha1.BlueGreenTrafficStrategyServiceSelectors
	if cluster.Spec.UpdateStrategy.BlueGreen != nil {
		if cluster.Spec.UpdateStrategy.BlueGreen.TrafficStrategy != "" {
			strategy = cluster.Spec.UpdateStrategy.BlueGreen.TrafficStrategy
		} else if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
			strategy = openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights
		}
	}

	ok, message := demotionPreconditionsSatisfied(
		greenSnapshots,
		int(cluster.Spec.Replicas),
		strategy,
		cluster.Status.BlueGreen.TrafficStep,
	)
	if !ok {
		logger.Info(message)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionDemoteBlueNonVotersStepDown, "demotion job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	// After demotion, verify Green is now the leader (merged from former Cutover phase)
	leaderPod, source, ok := m.findLeaderPod(ctx, logger, cluster, greenPods)
	if !ok {
		logger.Info("Green leader not yet elected after demotion, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to wait for leader election
	}

	logger.Info("Green leader confirmed after demotion", "pod", leaderPod, "source", source)

	// Transition to Cleanup
	return advance(openbaov1alpha1.PhaseCleanup), nil
}

// handlePhaseCleanup ejects Blue nodes from Raft and deletes the Blue StatefulSet.
// This is the "point of no return" - after this, rollback is not possible.
func (m *Manager) handlePhaseCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision

	// Raft safety gate: the Cleanup phase is the point of no return. Before we
	// remove Blue peers and delete Blue pods, ensure Green is stable and can
	// sustain quorum (all pods Ready+Unsealed, and a leader is observed).
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
		if _, source, ok := m.findLeaderPod(ctx, logger, cluster, greenPods); ok {
			leaderOK = true
			logger.V(1).Info("Green leader observed via API fallback", "source", source)
		}
	}

	ok, message := cleanupPreconditionsSatisfied(greenSnapshots, int(cluster.Spec.Replicas), leaderOK)
	if !ok {
		logger.Info(message)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	// Step 1: Eject Blue nodes from Raft peer list.
	// Note: Do not gate this on the service registration leader label; the executor
	// determines leadership via the health endpoint and can proceed even if labels lag.
	step, err := m.runExecutorJobStep(ctx, logger, cluster, ActionRemoveBluePeers, "cleanup peer removal job failure threshold exceeded")
	if err != nil {
		return phaseOutcome{}, err
	}
	if !step.Completed {
		return step.Outcome, nil
	}

	// Step 2: Delete Blue StatefulSet
	blueStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, blueRevision)
	blueStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      blueStatefulSetName,
	}, blueStatefulSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return phaseOutcome{}, fmt.Errorf("failed to get Blue StatefulSet: %w", err)
		}
		// Already deleted
		logger.Info("Blue StatefulSet already deleted", "blueRevision", blueRevision)
	} else {
		// Delete the StatefulSet - this will cascade-delete its pods
		if err := m.client.Delete(ctx, blueStatefulSet); err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to delete Blue StatefulSet: %w", err)
		}
		logger.Info("Deleted Blue StatefulSet", "blueRevision", blueRevision)
		return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to verify deletion and wait for pods to terminate
	}

	// Verify Blue pods are gone (excluding Terminating pods)
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to check Blue pods: %w", err)
	}

	// Filter out pods that are terminating (DeletionTimestamp is set)
	activeBluePods := 0
	for _, pod := range bluePods {
		if pod.DeletionTimestamp == nil {
			activeBluePods++
		}
	}

	if activeBluePods > 0 {
		logger.Info("Blue pods still exist, waiting for termination", "count", activeBluePods)
		return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to wait
	}

	// Finalize upgrade
	cluster.Status.CurrentVersion = cluster.Spec.Version
	cluster.Status.BlueGreen.BlueRevision = cluster.Status.BlueGreen.GreenRevision
	cluster.Status.BlueGreen.GreenRevision = ""
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle

	if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/upgrade", openbaov1alpha1.ClusterOperationUpgrade); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
		logger.Error(err, "Failed to release upgrade operation lock after blue/green completion")
	}

	logger.Info("Blue/green upgrade completed", "newVersion", cluster.Spec.Version)

	// Return a requeue to trigger another reconcile cycle so the InfraManager
	// can clean up the temporary -blue and -green Services used during GatewayWeights upgrades.
	return requeueAfterOutcome(constants.RequeueShort), nil
}

// checkAbortConditions checks if the upgrade should be aborted due to Green cluster failures.
// Returns (shouldAbort, error).
func (m *Manager) checkAbortConditions(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {

	// Only check abort conditions if we're past the DeployingGreen phase
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
		return false, nil
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	if greenRevision == "" {
		return false, nil
	}

	// Get Green pods
	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Green pods: %w", err)
	}

	// Check for CrashLoopBackOff or other failure states
	for _, pod := range greenPods {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					logger.Info("Green pod in failure state, aborting upgrade",
						"pod", pod.Name,
						"reason", reason)
					return true, nil
				}
			}
			if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
				logger.Info("Green pod terminated with error, aborting upgrade",
					"pod", pod.Name,
					"exitCode", containerStatus.State.Terminated.ExitCode)
				return true, nil
			}
		}
	}

	return false, nil
}

// abortUpgrade aborts the blue/green upgrade by cleaning up Green resources.
func (m *Manager) abortUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.BlueGreen == nil {
		return nil
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	if greenRevision == "" {
		// No Green cluster to abort
		return nil
	}

	logger.Info("Aborting blue/green upgrade", "greenRevision", greenRevision)

	// Delete Green StatefulSet
	if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
		return fmt.Errorf("failed to cleanup Green StatefulSet during abort: %w", err)
	}

	// Reset status
	cluster.Status.BlueGreen.GreenRevision = ""
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
	cluster.Status.BlueGreen.StartTime = nil
	cluster.Status.BlueGreen.TrafficSwitchedTime = nil
	cluster.Status.BlueGreen.TrafficStep = 0
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""

	logger.Info("Blue/green upgrade aborted successfully")

	return nil
}

// getMaxJobFailures returns the configured max job failures threshold or default (5).
func (m *Manager) getMaxJobFailures(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.MaxJobFailures != nil {
		return *cluster.Spec.UpdateStrategy.BlueGreen.MaxJobFailures
	}
	return 5 // Default
}

// isEarlyPhase returns true if the upgrade is in an early phase where abort (vs rollback) is appropriate.
func isEarlyPhase(phase openbaov1alpha1.BlueGreenPhase) bool {
	switch phase {
	case openbaov1alpha1.PhaseDeployingGreen, openbaov1alpha1.PhaseJoiningMesh, openbaov1alpha1.PhaseSyncing:
		return true
	default:
		return false
	}
}

// triggerRollbackOrAbort decides whether to abort (early phases) or trigger full rollback (late phases).
func (m *Manager) triggerRollbackOrAbort(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, reason string) (recon.Result, error) {
	phase := cluster.Status.BlueGreen.Phase

	if isEarlyPhase(phase) {
		// Early phase: simple abort (delete Green, reset to Idle)
		logger.Info("Aborting upgrade due to failures in early phase", "phase", phase, "reason", reason)
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to abort upgrade: %w", err)
		}
		return recon.Result{}, nil
	}

	// Late phase: full rollback required
	logger.Info("Triggering rollback due to failures in late phase", "phase", phase, "reason", reason)
	return m.triggerRollback(logger, cluster, reason)
}

// triggerRollback initiates rollback from any phase.
func (m *Manager) triggerRollback(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, reason string) (recon.Result, error) {
	now := metav1.Now()
	cluster.Status.BlueGreen.RollbackReason = reason
	cluster.Status.BlueGreen.RollbackStartTime = &now
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollingBack

	logger.Info("Rollback initiated", "reason", reason)

	return requeueShort(), nil // Requeue to process rollback
}

// handlePhaseTrafficSwitching handles the TrafficSwitching phase.
// This phase switches traffic to Green and optionally waits for stabilization.
func (m *Manager) handlePhaseTrafficSwitching(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision

	strategy := openbaov1alpha1.BlueGreenTrafficStrategyServiceSelectors
	if cluster.Spec.UpdateStrategy.BlueGreen != nil {
		if cluster.Spec.UpdateStrategy.BlueGreen.TrafficStrategy != "" {
			strategy = cluster.Spec.UpdateStrategy.BlueGreen.TrafficStrategy
		} else if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
			strategy = openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights
		}
	}

	// ServiceSelectors strategy keeps the original behavior: a single switch
	// followed by one stabilization period before demoting Blue.
	if strategy != openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights {
		return m.handlePhaseTrafficSwitchingServiceSelectors(ctx, logger, cluster, greenRevision)
	}

	return m.handlePhaseTrafficSwitchingGatewayWeights(ctx, logger, cluster, greenRevision)
}

// handlePhaseTrafficSwitchingServiceSelectors implements the original traffic switching
// behavior using the external Service selector.
func (m *Manager) handlePhaseTrafficSwitchingServiceSelectors(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, greenRevision string) (phaseOutcome, error) {
	autoRollback := autoRollbackSettings(cluster)
	stabilizationSeconds := autoRollback.StabilizationSeconds

	now := time.Now()

	var trafficSwitchedAt *time.Time
	var hookResult *JobResult
	greenHealthy := true

	if cluster.Status.BlueGreen.TrafficSwitchedTime != nil {
		switchedAt := cluster.Status.BlueGreen.TrafficSwitchedTime.Time
		trafficSwitchedAt = &switchedAt

		requiredDuration := time.Duration(stabilizationSeconds) * time.Second
		elapsed := now.Sub(switchedAt)

		if elapsed < requiredDuration {
			if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
				cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
				cluster.Spec.UpdateStrategy.BlueGreen.Verification.PostSwitchHook != nil {
				hook := cluster.Spec.UpdateStrategy.BlueGreen.Verification.PostSwitchHook
				result, err := m.ensurePostSwitchHookJob(ctx, logger, cluster, hook)
				if err != nil {
					return phaseOutcome{}, fmt.Errorf("failed to ensure post-switch hook job: %w", err)
				}
				hookResult = result
				if hookResult.Running {
					logger.Info("Post-switch hook job is in progress", "job", hookResult.Name)
				}
				if hookResult.Failed {
					logger.Info("Post-switch hook job failed", "job", hookResult.Name)
				}
				if hookResult.Succeeded {
					logger.Info("Post-switch hook completed successfully", "job", hookResult.Name)
				}
			}

			if autoRollback.Enabled && autoRollback.OnTrafficFailure {
				greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
				if err != nil {
					return phaseOutcome{}, fmt.Errorf("failed to get Green pods: %w", err)
				}
				for i := range greenPods {
					if !isPodReady(&greenPods[i]) {
						greenHealthy = false
						break
					}
				}
			}
		}
	}

	decision, err := trafficSwitchServiceSelectorsDecision(now, trafficSwitchedAt, stabilizationSeconds, autoRollback, hookResult, greenHealthy)
	if err != nil {
		return phaseOutcome{}, err
	}
	if decision.SetTrafficSwitchedAt {
		at := metav1.NewTime(decision.TrafficSwitchedAt)
		cluster.Status.BlueGreen.TrafficSwitchedTime = &at
		logger.Info("Traffic switched to Green", "greenRevision", greenRevision)
	}

	if decision.Outcome.kind == phaseOutcomeAdvance && decision.Outcome.nextPhase == openbaov1alpha1.PhaseDemotingBlue {
		logger.Info("Stabilization period complete, proceeding to demote Blue")
	}

	return decision.Outcome, nil
}

// handlePhaseTrafficSwitchingGatewayWeights implements weighted traffic shifting using
// Gateway API when the GatewayWeights traffic strategy is active. It advances through
// multiple traffic steps (e.g., 90/10 -> 50/50 -> 0/100), applying a stabilization
// period and validation at each step before proceeding to demote Blue.
func (m *Manager) handlePhaseTrafficSwitchingGatewayWeights(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, greenRevision string) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	// Stabilization period settings
	autoRollback := autoRollbackSettings(cluster)
	stabilizationSeconds := autoRollback.StabilizationSeconds

	now := time.Now()

	var trafficSwitchedAt *time.Time
	var hookResult *JobResult
	greenHealthy := true

	if cluster.Status.BlueGreen.TrafficSwitchedTime != nil {
		switchedAt := cluster.Status.BlueGreen.TrafficSwitchedTime.Time
		trafficSwitchedAt = &switchedAt

		requiredDuration := time.Duration(stabilizationSeconds) * time.Second
		elapsed := now.Sub(switchedAt)

		if elapsed < requiredDuration {
			if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
				cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
				cluster.Spec.UpdateStrategy.BlueGreen.Verification.PostSwitchHook != nil {
				hook := cluster.Spec.UpdateStrategy.BlueGreen.Verification.PostSwitchHook
				result, err := m.ensurePostSwitchHookJob(ctx, logger, cluster, hook)
				if err != nil {
					return phaseOutcome{}, fmt.Errorf("failed to ensure post-switch hook job: %w", err)
				}
				hookResult = result
				if hookResult.Running {
					logger.Info("Post-switch hook job is in progress", "job", hookResult.Name)
				}
				if hookResult.Failed {
					logger.Info("Post-switch hook job failed", "job", hookResult.Name)
				}
				if hookResult.Succeeded {
					logger.Info("Post-switch hook completed successfully", "job", hookResult.Name)
				}
			}

			if autoRollback.Enabled && autoRollback.OnTrafficFailure {
				greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
				if err != nil {
					return phaseOutcome{}, fmt.Errorf("failed to get Green pods: %w", err)
				}
				for i := range greenPods {
					if !isPodReady(&greenPods[i]) {
						greenHealthy = false
						break
					}
				}
			}
		}
	}

	decision, err := trafficSwitchGatewayWeightsDecision(
		now,
		cluster.Status.BlueGreen.TrafficStep,
		trafficSwitchedAt,
		stabilizationSeconds,
		autoRollback,
		hookResult,
		greenHealthy,
	)
	if err != nil {
		return phaseOutcome{}, err
	}

	if decision.SetTrafficStep {
		cluster.Status.BlueGreen.TrafficStep = decision.TrafficStep
	}
	if decision.SetTrafficSwitchedAt {
		at := metav1.NewTime(decision.TrafficSwitchedAt)
		cluster.Status.BlueGreen.TrafficSwitchedTime = &at
	}

	// Keep Gateway API HTTPRoute backends and weighted backend Services in sync
	// with the current traffic step. The workload controller does not reconcile
	// on every blue/green status update, so we apply these updates here.
	if m.infraManager != nil {
		if err := m.infraManager.EnsureGatewayTrafficResources(ctx, logger, cluster); err != nil {
			return phaseOutcome{}, fmt.Errorf("failed to reconcile Gateway traffic resources: %w", err)
		}
	}

	if decision.SetTrafficStep {
		switch decision.TrafficStep {
		case 1:
			logger.Info("Gateway weighted traffic: starting canary step", "trafficStep", cluster.Status.BlueGreen.TrafficStep)
		case 2:
			logger.Info("Gateway weighted traffic: advancing to mid-step", "trafficStep", cluster.Status.BlueGreen.TrafficStep)
		case 3:
			logger.Info("Gateway weighted traffic: advancing to final step", "trafficStep", cluster.Status.BlueGreen.TrafficStep)
		}
	}

	if decision.Outcome.kind == phaseOutcomeAdvance && decision.Outcome.nextPhase == openbaov1alpha1.PhaseDemotingBlue {
		logger.Info("Gateway weighted traffic stabilization complete, proceeding to demote Blue",
			"trafficStep", cluster.Status.BlueGreen.TrafficStep)
	}

	return decision.Outcome, nil
}

// handlePhaseRollingBack orchestrates the rollback sequence.
func (m *Manager) handlePhaseRollingBack(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision

	// Repair consensus in a single pass by ensuring Blue nodes are voters and
	// Green nodes are non-voters. This replaces the previous multi-step rollback
	// sequence and reduces the risk of leaving the cluster in a mixed state.
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
	)
	if err != nil {
		return phaseOutcome{}, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: repairing consensus", "job", result.Name)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}
	if result.Failed {
		// This is an expected failure path that intentionally halts automation. Avoid Error-level
		// logging here to prevent confusing stack traces in controller logs.
		logger.Info("Rollback consensus repair job failed; entering break glass mode", "job", result.Name)
		m.enterBreakGlassRollbackConsensusRepairFailed(cluster, result.Name)
		return hold(), nil
	}

	// Step 3: Verify Blue leader is elected
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Blue pods: %w", err)
	}

	leaderPod, source, ok := m.findLeaderPod(ctx, logger, cluster, bluePods)
	if !ok {
		logger.Info("Blue leader not yet elected during rollback, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to wait for leader election
	}

	logger.Info("Blue leader confirmed during rollback", "pod", leaderPod, "source", source)

	// Transition to RollbackCleanup
	return advance(openbaov1alpha1.PhaseRollbackCleanup), nil
}

// handlePhaseRollbackCleanup removes Green StatefulSet after rollback.
func (m *Manager) handlePhaseRollbackCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, fmt.Errorf("blue/green status is nil")
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision

	// Step 1: Remove Green peers from Raft
	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionRemoveBluePeers, // Reuse peer removal action, targeting Green handled by executor config
		rollbackRunID(cluster),
		blueRevision,
		greenRevision,
	)
	if err != nil {
		return phaseOutcome{}, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: removing Green peers", "job", result.Name)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}
	// Continue even if job failed - we still want to delete the StatefulSet

	// Step 2: Delete Green StatefulSet
	if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to cleanup Green StatefulSet during rollback: %w", err)
	}

	// Step 3: Verify Green pods are gone
	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to check Green pods: %w", err)
	}

	activeGreenPods := 0
	for _, pod := range greenPods {
		if pod.DeletionTimestamp == nil {
			activeGreenPods++
		}
	}

	if activeGreenPods > 0 {
		logger.Info("Green pods still exist during rollback cleanup, waiting", "count", activeGreenPods)
		return requeueAfterOutcome(constants.RequeueShort), nil // Requeue to wait
	}

	// Finalize rollback
	rollbackReason := cluster.Status.BlueGreen.RollbackReason
	cluster.Status.BlueGreen.GreenRevision = ""
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
	cluster.Status.BlueGreen.StartTime = nil
	cluster.Status.BlueGreen.TrafficSwitchedTime = nil
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""
	// Keep RollbackReason and RollbackStartTime for observability

	logger.Info("Blue/green rollback completed", "reason", rollbackReason)

	return advance(openbaov1alpha1.PhaseIdle), nil
}
