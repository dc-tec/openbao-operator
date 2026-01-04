package bluegreen

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	configbuilder "github.com/dc-tec/openbao-operator/internal/config"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/infra"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/revision"
	"github.com/dc-tec/openbao-operator/internal/security"
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

func imageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ImageVerification == nil {
		return constants.ImageVerificationFailurePolicyBlock
	}
	failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
	if failurePolicy == "" {
		return constants.ImageVerificationFailurePolicyBlock
	}
	return failurePolicy
}

func initContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.InitContainer == nil {
		return ""
	}
	return cluster.Spec.InitContainer.Image
}

func (m *Manager) verifyImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}
	if imageRef == "" {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := security.VerifyImageForCluster(verifyCtx, logger, m.client, cluster, imageRef)
	if err == nil {
		logger.Info("Image verified successfully", "digest", digest)
		return digest, nil
	}

	failurePolicy := imageVerificationFailurePolicy(cluster)
	if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
		return "", operatorerrors.WithReason(failureReason, fmt.Errorf("%s (policy=Block): %w", failureMessagePrefix, err))
	}

	logger.Error(err, failureMessagePrefix+" but proceeding due to Warn policy", "image", imageRef)
	return "", nil
}

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
	// Check if blue/green strategy is enabled
	if cluster.Spec.UpdateStrategy.Type != openbaov1alpha1.UpdateStrategyBlueGreen {
		logger.V(1).Info("UpdateStrategy is not BlueGreen; skipping blue/green upgrade reconciliation",
			"updateStrategy", cluster.Spec.UpdateStrategy.Type)
		return recon.Result{}, nil
	}

	// Check if cluster is initialized; upgrades can only proceed after initialization
	if !cluster.Status.Initialized {
		logger.Info("Cluster not initialized; skipping blue/green upgrade reconciliation")
		return requeueStandard(), nil
	}

	// Ensure upgrade ServiceAccount exists (for JWT Auth)
	// This is required before any OpenBao API calls that need authentication
	if err := m.ensureUpgradeServiceAccount(ctx, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	// Initialize BlueGreen status if needed
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
	} else if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle &&
		(cluster.Status.BlueGreen.BlueRevision == "" || cluster.Status.CurrentVersion != cluster.Spec.Version) {
		// If the operator restarted and BlueRevision was derived from spec (target) rather than the
		// currently running ("Blue") pods, correct it before starting a new upgrade or snapshot.
		inferred, err := infra.InferActiveRevisionFromPods(ctx, m.client, cluster)
		if err != nil {
			logger.Error(err, "Failed to infer active revision from pods; keeping existing BlueRevision", "blueRevision", cluster.Status.BlueGreen.BlueRevision)
		} else if inferred != "" && inferred != cluster.Status.BlueGreen.BlueRevision {
			logger.Info("Correcting BlueRevision from active pods", "from", cluster.Status.BlueGreen.BlueRevision, "to", inferred)
			cluster.Status.BlueGreen.BlueRevision = inferred
		}
	}

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active && cluster.Spec.BreakGlassAck != cluster.Status.BreakGlass.Nonce {
		logger.Info("Cluster is in break glass mode; halting blue/green reconciliation",
			"breakGlassReason", cluster.Status.BreakGlass.Reason,
			"breakGlassNonce", cluster.Status.BreakGlass.Nonce)
		return requeueStandard(), nil
	}

	upgradeActive := cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle
	upgradeNeeded := cluster.Status.CurrentVersion != "" && cluster.Spec.Version != cluster.Status.CurrentVersion
	if upgradeActive || upgradeNeeded {
		if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
			Holder:    constants.ControllerNameOpenBaoCluster + "/upgrade",
			Operation: openbaov1alpha1.ClusterOperationUpgrade,
			Message:   fmt.Sprintf("blue/green upgrade phase %s", cluster.Status.BlueGreen.Phase),
		}); err != nil {
			if errors.Is(err, operationlock.ErrLockHeld) {
				if upgradeActive {
					return recon.Result{}, fmt.Errorf("blue/green upgrade in progress but operation lock is held by another operation: %w", err)
				}
				logger.Info("Blue/green upgrade blocked by operation lock", "error", err.Error())
				return requeueStandard(), nil
			}
			return recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
		}
	}

	// Check if upgrade is needed
	// CRITICAL: If CurrentVersion is empty, the cluster is in initial state (not yet set by controller).
	// We should NOT trigger an upgrade until CurrentVersion is set. An upgrade is only needed when
	// CurrentVersion is set AND different from Spec.Version.
	if cluster.Status.CurrentVersion == "" {
		// Cluster is in initial state, CurrentVersion not yet set. No upgrade needed.
		logger.Info("CurrentVersion not yet set; waiting for initial version to be established")
		// Ensure we're in Idle phase
		if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
			// Cleanup any lingering Green StatefulSet
			if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
				return recon.Result{}, fmt.Errorf("failed to cleanup Green StatefulSet: %w", err)
			}
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
			cluster.Status.BlueGreen.GreenRevision = ""
			cluster.Status.BlueGreen.StartTime = nil
		}
		return requeueStandard(), nil
	}

	if cluster.Status.CurrentVersion == cluster.Spec.Version {
		// No upgrade needed, ensure we're in Idle phase
		logger.V(1).Info("No upgrade needed; CurrentVersion matches Spec.Version",
			"currentVersion", cluster.Status.CurrentVersion,
			"specVersion", cluster.Spec.Version)
		if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
			// Cleanup any lingering Green StatefulSet
			if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
				return recon.Result{}, fmt.Errorf("failed to cleanup Green StatefulSet: %w", err)
			}
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
			cluster.Status.BlueGreen.GreenRevision = ""
			cluster.Status.BlueGreen.StartTime = nil
		}
		return recon.Result{}, nil
	}

	logger.Info("Upgrade detected; CurrentVersion differs from Spec.Version",
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version)

	// Check for abort conditions (Green cluster failures)
	if shouldAbort, err := m.checkAbortConditions(ctx, logger, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to check abort conditions: %w", err)
	} else if shouldAbort {
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to abort upgrade: %w", err)
		}
		return requeueShort(), nil
	}

	// Upgrade is needed - execute state machine
	return m.executeStateMachine(ctx, logger, cluster, verifiedImageDigest)
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
		verifiedInitContainerDigest, err := m.verifyImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Green init container image verification failed")
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

	if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/upgrade", openbaov1alpha1.ClusterOperationUpgrade); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
		logger.Error(err, "Failed to release upgrade operation lock after blue/green completion")
	}

	logger.Info("Blue/green upgrade completed", "newVersion", cluster.Spec.Version)

	return advance(openbaov1alpha1.PhaseIdle), nil
}

func (m *Manager) handleBreakGlassAck(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (handled bool, result recon.Result) {
	if cluster.Status.BreakGlass == nil || !cluster.Status.BreakGlass.Active {
		return false, recon.Result{}
	}

	if cluster.Status.BreakGlass.Nonce == "" || cluster.Spec.BreakGlassAck != cluster.Status.BreakGlass.Nonce {
		return true, recon.Result{}
	}

	now := metav1.Now()
	cluster.Status.BreakGlass.Active = false
	cluster.Status.BreakGlass.AcknowledgedAt = &now

	logger.Info("Break glass acknowledged; resuming automation", "reason", cluster.Status.BreakGlass.Reason)

	if cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseRollingBack &&
		cluster.Status.BreakGlass.Reason == openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed {
		cluster.Status.BlueGreen.RollbackAttempt++
		cluster.Status.BlueGreen.JobFailureCount = 0
		cluster.Status.BlueGreen.LastJobFailure = ""
		return true, requeueShort()
	}

	return true, requeueShort()
}

const breakGlassNonceBytes = 16

func rollbackRunID(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.RollbackAttempt <= 0 {
		return "rollback"
	}
	return fmt.Sprintf("rollback-retry-%d", cluster.Status.BlueGreen.RollbackAttempt)
}

func (m *Manager) enterBreakGlassRollbackConsensusRepairFailed(cluster *openbaov1alpha1.OpenBaoCluster, jobName string) {
	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active {
		return
	}

	now := metav1.Now()

	nonce := newBreakGlassNonce()
	message := fmt.Sprintf("Rollback consensus repair Job %s failed; manual intervention required", jobName)

	steps := []string{
		fmt.Sprintf("Inspect rollback Job logs: kubectl -n %s logs job/%s", cluster.Namespace, jobName),
		fmt.Sprintf("Inspect pod status: kubectl -n %s get pods -l %s=%s -o wide", cluster.Namespace, constants.LabelOpenBaoCluster, cluster.Name),
		fmt.Sprintf("Attempt to identify Raft leader: kubectl -n %s exec -it %s-0 -- bao operator raft list-peers", cluster.Namespace, cluster.Name),
		"Perform any required Raft recovery steps (peer removal, snapshot restore) per the user-guide runbook.",
		fmt.Sprintf("Acknowledge break glass to retry rollback automation: kubectl -n %s patch openbaocluster %s --type merge -p '{\"spec\":{\"breakGlassAck\":\"%s\"}}'", cluster.Namespace, cluster.Name, nonce),
	}

	cluster.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{
		Active:    true,
		Reason:    openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
		Message:   message,
		Nonce:     nonce,
		EnteredAt: &now,
		Steps:     steps,
	}

}

func newBreakGlassNonce() string {
	buf := make([]byte, breakGlassNonceBytes)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

func (m *Manager) getPodURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	return fmt.Sprintf("https://%s.%s.%s.svc:%d", podName, cluster.Name, cluster.Namespace, constants.PortAPI)
}

func (m *Manager) getClusterCACert(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	for _, suffix := range []string{constants.SuffixTLSCA, constants.SuffixTLSServer} {
		secretName := cluster.Name + suffix
		secret := &corev1.Secret{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      secretName,
		}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get CA secret %s/%s: %w", cluster.Namespace, secretName, err)
		}

		caCert, ok := secret.Data["ca.crt"]
		if !ok {
			return nil, fmt.Errorf("CA certificate not found in secret %s/%s", cluster.Namespace, secretName)
		}
		return caCert, nil
	}

	return nil, fmt.Errorf("no CA secret found for cluster %s/%s (tried %q and %q)", cluster.Namespace, cluster.Name, cluster.Name+constants.SuffixTLSCA, cluster.Name+constants.SuffixTLSServer)
}

func (m *Manager) findLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool) {
	// Fast path: trust leader label if it is set.
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}

		active, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil {
			logger.V(1).Info("Invalid OpenBao leader label value", "pod", pod.Name, "error", err)
			continue
		}
		if present && active {
			return pod.Name, "label", true
		}
	}

	// Fallback: query /v1/sys/health to determine leadership (labels can lag).
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		logger.V(1).Info("Failed to load cluster CA certificate; cannot use API leader fallback", "error", err)
		return "", "", false
	}

	clusterKey := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if !isPodReady(pod) {
			continue
		}

		sealed, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
		if err == nil && present && sealed {
			continue
		}

		apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
			ClusterKey:          clusterKey,
			BaseURL:             m.getPodURL(cluster, pod.Name),
			CACert:              caCert,
			ConnectionTimeout:   2 * time.Second,
			RequestTimeout:      2 * time.Second,
			SmartClientDisabled: true,
		})
		if err != nil {
			logger.V(1).Info("Failed to create OpenBao client for pod", "pod", pod.Name, "error", err)
			continue
		}

		isLeader, err := apiClient.IsLeader(ctx)
		if err != nil {
			logger.V(1).Info("Leader check failed for pod", "pod", pod.Name, "error", err)
			continue
		}
		if isLeader {
			return pod.Name, "api", true
		}
	}

	return "", "", false
}

// getPodsByRevision returns all pods belonging to the specified revision.
// This is a unified helper used for both Blue and Green pod lookup.
func (m *Manager) getPodsByRevision(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, rev string) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:     cluster.Name,
		constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
		constants.LabelOpenBaoRevision: rev,
	})

	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, fmt.Errorf("failed to list pods for revision %s: %w", rev, err)
	}

	return podList.Items, nil
}

// getGreenPods returns all pods belonging to the Green revision (convenience wrapper).
func (m *Manager) getGreenPods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, greenRevision string) ([]corev1.Pod, error) {
	return m.getPodsByRevision(ctx, cluster, greenRevision)
}

// cleanupGreenStatefulSet removes any lingering Green StatefulSet.
func (m *Manager) cleanupGreenStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.GreenRevision == "" {
		return nil
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	greenStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      fmt.Sprintf("%s-%s", cluster.Name, greenRevision),
	}, greenStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to get Green StatefulSet: %w", err)
	}

	if err := m.client.Delete(ctx, greenStatefulSet); err != nil {
		return fmt.Errorf("failed to delete Green StatefulSet: %w", err)
	}

	logger.Info("Cleaned up Green StatefulSet", "greenRevision", greenRevision)
	return nil
}

// ensureUpgradeServiceAccount creates or updates the ServiceAccount for upgrade operations using Server-Side Apply.
// This ServiceAccount is used by upgrade executor Jobs for JWT Auth authentication to OpenBao.
func (m *Manager) ensureUpgradeServiceAccount(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := cluster.Name + constants.SuffixUpgradeServiceAccount

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: "upgrade",
			},
		},
	}

	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-operator"),
	}

	if err := m.client.Patch(ctx, sa, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to ensure upgrade ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// getBluePods returns all pods belonging to the Blue revision (convenience wrapper).
func (m *Manager) getBluePods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, blueRevision string) ([]corev1.Pod, error) {
	return m.getPodsByRevision(ctx, cluster, blueRevision)
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
	result, err := EnsureExecutorJob(
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
	result, err := EnsureExecutorJob(
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

// isPodReady checks if a pod is in Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (m *Manager) ensureValidationHookJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	component string,
	hook *openbaov1alpha1.ValidationHookConfig,
) (*JobResult, error) {
	if hook == nil {
		return nil, fmt.Errorf("hook is required")
	}
	if hook.Image == "" {
		return nil, fmt.Errorf("hook.image is required")
	}

	timeout := int32(300)
	if hook.TimeoutSeconds != nil {
		timeout = *hook.TimeoutSeconds
	}
	backoffLimit := int32(0)
	ttlSeconds := ptr.To(int32(jobTTLSeconds))

	return ensureJob(ctx, m.client, m.scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      jobName,
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
					constants.LabelAppInstance:      cluster.Name,
					constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
					constants.LabelOpenBaoCluster:   cluster.Name,
					constants.LabelOpenBaoComponent: component,
				},
			},
			Spec: batchv1.JobSpec{
				BackoffLimit:            &backoffLimit,
				ActiveDeadlineSeconds:   ptr.To(int64(timeout)),
				TTLSecondsAfterFinished: ttlSeconds,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{
							{
								Name:    "validation",
								Image:   hook.Image,
								Command: hook.Command,
								Args:    hook.Args,
							},
						},
					},
				},
			},
		}, nil
	}, "component", component)
}

func (m *Manager) ensurePrePromotionHookJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	hook *openbaov1alpha1.ValidationHookConfig,
) (*JobResult, error) {
	return m.ensureValidationHookJob(ctx, logger, cluster, fmt.Sprintf("%s-validation-hook", cluster.Name), ComponentValidationHook, hook)
}

func (m *Manager) ensurePostSwitchHookJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	hook *openbaov1alpha1.ValidationHookConfig,
) (*JobResult, error) {
	return m.ensureValidationHookJob(ctx, logger, cluster, fmt.Sprintf("%s-post-switch-validation-hook", cluster.Name), ComponentPostSwitchValidationHook, hook)
}

// ensurePreUpgradeSnapshotJob creates or checks the status of the pre-upgrade snapshot Job.
func (m *Manager) ensurePreUpgradeSnapshotJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (*JobResult, error) {
	if cluster.Spec.Backup == nil || cluster.Spec.Backup.Target.Endpoint == "" {
		return nil, fmt.Errorf("backup configuration required for pre-upgrade snapshot")
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		return nil, operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so snapshot Jobs can reach the object storage endpoint",
			)),
		)
	}

	if cluster.Spec.Backup.ExecutorImage == "" {
		return nil, fmt.Errorf("backup executor image is required for snapshots")
	}

	if err := backup.EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster); err != nil {
		return nil, fmt.Errorf("failed to ensure backup ServiceAccount for snapshot job: %w", err)
	}
	if err := backup.EnsureBackupRBAC(ctx, m.client, m.scheme, cluster); err != nil {
		return nil, fmt.Errorf("failed to ensure backup RBAC for snapshot job: %w", err)
	}

	return ensureJob(ctx, m.client, m.scheme, logger, cluster, jobName, func(jobName string) (*batchv1.Job, error) {
		verifiedExecutorDigest, err := m.verifyImageDigest(
			ctx,
			logger,
			cluster,
			cluster.Spec.Backup.ExecutorImage,
			constants.ReasonBlueGreenSnapshotImageVerificationFailed,
			"Pre-upgrade snapshot executor image verification failed",
		)
		if err != nil {
			return nil, err
		}
		return m.buildSnapshotJob(cluster, jobName, "pre-upgrade", verifiedExecutorDigest), nil
	}, "component", ComponentUpgradeSnapshot, "phase", "pre-upgrade")
}

// buildSnapshotJob creates a backup Job spec for upgrade snapshots.
func (m *Manager) buildSnapshotJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName, phase string, verifiedExecutorDigest string) *batchv1.Job {
	region := cluster.Spec.Backup.Target.Region
	if region == "" {
		region = "us-east-1"
	}

	// Compute StatefulSet name for pod discovery
	// For pre-upgrade snapshots, use the Blue revision (current stable pods)
	statefulSetName := cluster.Name
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
		statefulSetName = fmt.Sprintf("%s-%s", cluster.Name, cluster.Status.BlueGreen.BlueRevision)
	}

	// Build environment variables for the backup container
	env := []corev1.EnvVar{
		{Name: constants.EnvClusterNamespace, Value: cluster.Namespace},
		{Name: constants.EnvClusterName, Value: cluster.Name},
		{Name: constants.EnvStatefulSetName, Value: statefulSetName},
		{Name: constants.EnvClusterReplicas, Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: constants.EnvBackupEndpoint, Value: cluster.Spec.Backup.Target.Endpoint},
		{Name: constants.EnvBackupBucket, Value: cluster.Spec.Backup.Target.Bucket},
		{Name: constants.EnvBackupPathPrefix, Value: cluster.Spec.Backup.Target.PathPrefix},
		{Name: constants.EnvBackupRegion, Value: region},
		{Name: constants.EnvBackupUsePathStyle, Value: fmt.Sprintf("%t", cluster.Spec.Backup.Target.UsePathStyle)},
	}

	// Add role ARN if configured
	if cluster.Spec.Backup.Target.RoleARN != "" {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvAWSRoleARN,
			Value: cluster.Spec.Backup.Target.RoleARN,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvAWSWebIdentityTokenFile,
			Value: "/var/run/secrets/aws/token",
		})
	}

	// Add JWT Auth configuration if available
	if cluster.Spec.Backup.JWTAuthRole != "" {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupJWTAuthRole,
			Value: cluster.Spec.Backup.JWTAuthRole,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodJWT,
		})
	}

	backoffLimit := int32(0) // Don't retry
	ttlSecondsAfterFinished := int32(3600)

	// Build volume mounts
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tls-ca",
			MountPath: constants.PathTLS,
			ReadOnly:  true,
		},
	}

	// Add JWT token mount if configured
	if cluster.Spec.Backup.JWTAuthRole != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "openbao-token",
			MountPath: "/var/run/secrets/tokens",
			ReadOnly:  true,
		})
	}

	// Add credentials secret mount if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "backup-credentials",
			MountPath: constants.PathBackupCredentials,
			ReadOnly:  true,
		})
	}

	// Build volumes
	volumes := []corev1.Volume{
		{
			Name: "tls-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cluster.Name + constants.SuffixTLSCA,
				},
			},
		},
	}

	// Add JWT token volume if configured
	if cluster.Spec.Backup.JWTAuthRole != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "openbao-token",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              "openbao-token",
								ExpirationSeconds: ptr.To(int64(3600)),
								Audience:          constants.TokenAudienceOpenBaoInternal,
							},
						},
					},
				},
			},
		})
	}

	// Add credentials secret volume if provided
	if cluster.Spec.Backup.Target.CredentialsSecretRef != nil {
		credentialsFileMode := int32(0400) // Owner read-only
		volumes = append(volumes, corev1.Volume{
			Name: "backup-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  cluster.Spec.Backup.Target.CredentialsSecretRef.Name,
					DefaultMode: &credentialsFileMode,
				},
			},
		})
	}

	// Service account name - use backup service account
	serviceAccountName := cluster.Name + constants.SuffixBackupServiceAccount

	image := verifiedExecutorDigest
	if image == "" {
		image = cluster.Spec.Backup.ExecutorImage
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: ComponentUpgradeSnapshot,
			},
			Annotations: map[string]string{
				AnnotationSnapshotPhase: phase,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster")),
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
						constants.LabelAppInstance:      cluster.Name,
						constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
						constants.LabelOpenBaoCluster:   cluster.Name,
						constants.LabelOpenBaoComponent: ComponentUpgradeSnapshot,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           serviceAccountName,
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(constants.UserBackup),
						RunAsGroup:   ptr.To(constants.GroupBackup),
						FSGroup:      ptr.To(constants.GroupBackup),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "backup",
							Image: image,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								ReadOnlyRootFilesystem: ptr.To(true),
								RunAsNonRoot:           ptr.To(true),
							},
							Env:          env,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	return job
}
