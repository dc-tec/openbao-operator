package bluegreen

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	configbuilder "github.com/openbao/operator/internal/config"
	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/internal/infra"
	openbaoapi "github.com/openbao/operator/internal/openbao"
	"github.com/openbao/operator/internal/revision"
)

var (
	// ErrBlueGreenNotConfigured indicates that blue/green upgrade strategy is not configured.
	ErrBlueGreenNotConfigured = errors.New("blue/green upgrade strategy not configured")
	// ErrRevisionCalculationFailed indicates that revision calculation failed.
	ErrRevisionCalculationFailed = errors.New("revision calculation failed")
)

// Manager manages blue/green upgrade operations for OpenBaoCluster.
type Manager struct {
	client       client.Client
	scheme       *runtime.Scheme
	infraManager *infra.Manager
}

// NewManager constructs a Manager.
func NewManager(c client.Client, scheme *runtime.Scheme, infraManager *infra.Manager) *Manager {
	return &Manager{
		client:       c,
		scheme:       scheme,
		infraManager: infraManager,
	}
}

// Reconcile manages the blue/green upgrade state machine.
// This implements the SubReconciler interface.
// Returns (shouldRequeue, error) where shouldRequeue indicates if reconciliation should be requeued immediately.
//
// Note: Image verification is handled by the infra reconciler which runs before this.
// For Green StatefulSet creation, we use cluster.Spec.Image. If image verification
// is enabled, the infra reconciler will have already verified it.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
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

	// Use spec image (infra reconciler handles verification)
	verifiedImageDigest := cluster.Spec.Image

	return m.reconcileBlueGreen(ctx, logger, cluster, verifiedImageDigest)
}

// reconcileBlueGreen is the internal reconcile method that handles blue/green upgrades.
func (m *Manager) reconcileBlueGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) (bool, error) {
	// Check if blue/green strategy is enabled
	if cluster.Spec.UpdateStrategy.Type != openbaov1alpha1.UpdateStrategyBlueGreen {
		logger.V(1).Info("UpdateStrategy is not BlueGreen; skipping blue/green upgrade reconciliation",
			"updateStrategy", cluster.Spec.UpdateStrategy.Type)
		return false, nil
	}

	// Check if cluster is initialized; upgrades can only proceed after initialization
	if !cluster.Status.Initialized {
		logger.Info("Cluster not initialized; skipping blue/green upgrade reconciliation")
		return false, nil
	}

	// Ensure upgrade ServiceAccount exists (for JWT Auth)
	// This is required before any OpenBao API calls that need authentication
	if err := m.ensureUpgradeServiceAccount(ctx, cluster); err != nil {
		return false, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	// Initialize BlueGreen status if needed
	if cluster.Status.BlueGreen == nil {
		cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase:        openbaov1alpha1.PhaseIdle,
			BlueRevision: m.calculateRevision(cluster),
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
				return false, fmt.Errorf("failed to cleanup Green StatefulSet: %w", err)
			}
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
			cluster.Status.BlueGreen.GreenRevision = ""
			cluster.Status.BlueGreen.StartTime = nil
		}
		return false, nil
	}

	if cluster.Status.CurrentVersion == cluster.Spec.Version {
		// No upgrade needed, ensure we're in Idle phase
		logger.V(1).Info("No upgrade needed; CurrentVersion matches Spec.Version",
			"currentVersion", cluster.Status.CurrentVersion,
			"specVersion", cluster.Spec.Version)
		if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
			// Cleanup any lingering Green StatefulSet
			if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
				return false, fmt.Errorf("failed to cleanup Green StatefulSet: %w", err)
			}
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
			cluster.Status.BlueGreen.GreenRevision = ""
			cluster.Status.BlueGreen.StartTime = nil
		}
		return false, nil
	}

	logger.Info("Upgrade detected; CurrentVersion differs from Spec.Version",
		"currentVersion", cluster.Status.CurrentVersion,
		"specVersion", cluster.Spec.Version)

	// Check for abort conditions (Green cluster failures)
	if shouldAbort, err := m.checkAbortConditions(ctx, logger, cluster); err != nil {
		return false, fmt.Errorf("failed to check abort conditions: %w", err)
	} else if shouldAbort {
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return false, fmt.Errorf("failed to abort upgrade: %w", err)
		}
		return false, nil
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
	now := metav1.Now()
	cluster.Status.BlueGreen.StartTime = &now
	// Reset job failure count on phase transition
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""
}

// executeStateMachine runs the blue/green upgrade state machine.
func (m *Manager) executeStateMachine(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) (bool, error) {
	phase := cluster.Status.BlueGreen.Phase

	logger = logger.WithValues("phase", phase)

	switch phase {
	case openbaov1alpha1.PhaseIdle:
		return m.handlePhaseIdle(ctx, logger, cluster, verifiedImageDigest)
	case openbaov1alpha1.PhaseDeployingGreen:
		return m.handlePhaseDeployingGreen(ctx, logger, cluster, verifiedImageDigest)
	case openbaov1alpha1.PhaseJoiningMesh:
		return m.handlePhaseJoiningMesh(ctx, logger, cluster)
	case openbaov1alpha1.PhaseSyncing:
		return m.handlePhaseSyncing(ctx, logger, cluster)
	case openbaov1alpha1.PhasePromoting:
		return m.handlePhasePromoting(ctx, logger, cluster)
	case openbaov1alpha1.PhaseTrafficSwitching:
		return m.handlePhaseTrafficSwitching(ctx, logger, cluster)
	case openbaov1alpha1.PhaseDemotingBlue:
		return m.handlePhaseDemotingBlue(ctx, logger, cluster)
	case openbaov1alpha1.PhaseCleanup:
		return m.handlePhaseCleanup(ctx, logger, cluster)
	case openbaov1alpha1.PhaseRollingBack:
		return m.handlePhaseRollingBack(ctx, logger, cluster)
	case openbaov1alpha1.PhaseRollbackCleanup:
		return m.handlePhaseRollbackCleanup(ctx, logger, cluster)
	default:
		return false, fmt.Errorf("unknown blue/green phase: %s", phase)
	}
}

// handlePhaseIdle transitions from Idle to DeployingGreen when an upgrade is detected.
func (m *Manager) handlePhaseIdle(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (bool, error) {
	logger.Info("DEBUG: handlePhaseIdle entered",
		"PreUpgradeSnapshot", cluster.Spec.UpdateStrategy.BlueGreen.PreUpgradeSnapshot,
		"SnapshotJobName", cluster.Status.BlueGreen.PreUpgradeSnapshotJobName)
	logger.Info("Starting blue/green upgrade",
		"fromVersion", cluster.Status.CurrentVersion,
		"targetVersion", cluster.Spec.Version)

	// Pre-upgrade snapshot (if enabled)
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.PreUpgradeSnapshot {
		// Check if snapshot job is already tracked
		if cluster.Status.BlueGreen.PreUpgradeSnapshotJobName == "" {
			// Create the pre-upgrade snapshot job
			jobName, err := m.createPreUpgradeSnapshotJob(ctx, logger, cluster)
			if err != nil {
				logger.Error(err, "Failed to create pre-upgrade snapshot job")
				fmt.Printf("DEBUG: Failed to create pre-upgrade snapshot job: %v\n", err)
				return false, err // Block upgrade on snapshot failure
			} else {
				cluster.Status.BlueGreen.PreUpgradeSnapshotJobName = jobName
				logger.Info("Pre-upgrade snapshot job created", "job", jobName)
				return true, nil // Requeue to wait for snapshot
			}
		} else {
			// Check if snapshot job completed
			job := &batchv1.Job{}
			err := m.client.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      cluster.Status.BlueGreen.PreUpgradeSnapshotJobName,
			}, job)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Failed to check pre-upgrade snapshot job status")
				}
				// Job gone or error - continue with upgrade
			} else if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
				// Job still running
				logger.Info("Waiting for pre-upgrade snapshot to complete",
					"job", cluster.Status.BlueGreen.PreUpgradeSnapshotJobName)
				return true, nil // Requeue to wait
			}
			logger.Info("Pre-upgrade snapshot completed",
				"job", cluster.Status.BlueGreen.PreUpgradeSnapshotJobName)
		}
	}

	// Set upgrading condition
	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeStarted,
		Message:            fmt.Sprintf("Blue/green upgrade from %s to %s has started", cluster.Status.CurrentVersion, cluster.Spec.Version),
	})

	// Calculate Green revision
	greenRevision := m.calculateRevision(cluster)
	cluster.Status.BlueGreen.GreenRevision = greenRevision
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseDeployingGreen
	cluster.Status.BlueGreen.StartTime = &now

	return true, nil // Requeue to proceed to next phase
}

// handlePhaseDeployingGreen creates the Green StatefulSet.
// IMPORTANT: Green pods must join the existing Blue cluster as non-voters, not initialize a new cluster.
func (m *Manager) handlePhaseDeployingGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (bool, error) {
	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision
	logger = logger.WithValues("greenRevision", greenRevision, "blueRevision", blueRevision)

	// CRITICAL: Before creating Green StatefulSet, ensure Blue pods are ready and available.
	// Green pods must join an existing initialized cluster, not form a new one.
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Blue pods: %w", err)
	}

	if len(bluePods) == 0 {
		logger.Info("No Blue pods found yet, waiting...")
		return true, nil // Requeue to wait for Blue pods
	}

	// Verify Blue pods have the revision label (required for retry_join to work)
	// If Blue pods don't have the revision label, the InfraManager needs to update them first
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
		// Trigger InfraManager reconciliation to update Blue StatefulSet with revision label
		// This will happen automatically on next reconcile, but we can requeue to speed it up
		logger.Info("Blue pods missing revision label; waiting for InfraManager to update StatefulSet")
		return true, nil // Requeue to wait for Blue StatefulSet to be updated with revision label
	}

	// Verify at least one Blue pod is ready and unsealed
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
		return true, nil // Requeue to wait for Blue pods to be ready
	}

	// Check if Green StatefulSet already exists
	greenStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, greenRevision)
	greenStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      greenStatefulSetName,
	}, greenStatefulSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get Green StatefulSet: %w", err)
		}

		// StatefulSet doesn't exist, create it
		// Render config for Green cluster (same as Blue, but different revision)
		// Use the same headless service name as Blue (they share the same service)
		// CRITICAL: Set TargetRevisionForJoin to Blue revision so Green pods only
		// discover Blue pods via retry_join, preventing them from forming their own cluster.
		// Green pods will use retry_join to discover Blue pods and join as non-voters.
		infraDetails := configbuilder.InfrastructureDetails{
			HeadlessServiceName:   cluster.Name, // Headless service name is cluster name
			Namespace:             cluster.Namespace,
			APIPort:               constants.PortAPI,
			ClusterPort:           constants.PortCluster,
			TargetRevisionForJoin: blueRevision, // Green pods should only discover Blue pods
		}

		renderedConfig, err := configbuilder.RenderHCL(cluster, infraDetails)
		if err != nil {
			return false, fmt.Errorf("failed to render config for Green cluster: %w", err)
		}

		configContent := string(renderedConfig)

		// Create Green StatefulSet with revision
		// Note: Green StatefulSet should start with all replicas immediately (no rolling)
		// Use cluster.Spec.Image directly to ensure we get the target version (not the captured verifiedImageDigest)
		greenImage := cluster.Spec.Image
		if err := m.infraManager.EnsureStatefulSetWithRevision(ctx, logger, cluster, configContent, greenImage, greenRevision, true); err != nil {
			return false, fmt.Errorf("failed to create Green StatefulSet: %w", err)
		}

		logger.Info("Created Green StatefulSet", "greenRevision", greenRevision)
		return true, nil // Requeue to wait for StatefulSet to be created
	}

	// StatefulSet exists, check if all replicas are ready
	desiredReplicas := cluster.Spec.Replicas
	if greenStatefulSet.Spec.Replicas != nil {
		desiredReplicas = *greenStatefulSet.Spec.Replicas
	}

	if greenStatefulSet.Status.ReadyReplicas < desiredReplicas {
		logger.Info("Waiting for Green pods to be ready",
			"readyReplicas", greenStatefulSet.Status.ReadyReplicas,
			"desiredReplicas", desiredReplicas)
		return true, nil // Requeue to wait for pods
	}

	// Check if pods are actually running (not just ready in StatefulSet status)
	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Green pods: %w", err)
	}

	allRunning := true
	for _, pod := range greenPods {
		if pod.Status.Phase != corev1.PodRunning {
			logger.Info("Green pod not yet running", "pod", pod.Name, "phase", pod.Status.Phase)
			allRunning = false
			break
		}
	}

	if !allRunning {
		return true, nil // Requeue to wait
	}

	// Check if all Green pods are unsealed (merged from former UnsealingGreen phase)
	for _, pod := range greenPods {
		sealed, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
		if err != nil {
			return false, fmt.Errorf("failed to parse sealed label on pod %s: %w", pod.Name, err)
		}
		if !present || sealed {
			logger.Info("Waiting for Green pod to be unsealed", "pod", pod.Name)
			return true, nil // Requeue to wait for unseal
		}
	}

	// All pods are ready, running, and unsealed - transition to JoiningMesh
	m.transitionToPhase(cluster, openbaov1alpha1.PhaseJoiningMesh)

	return true, nil
}

// handlePhaseJoiningMesh joins Green pods to the Raft cluster as non-voters.
func (m *Manager) handlePhaseJoiningMesh(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision

	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionJoinGreenNonVoters,
		"",
		blueRevision,
		greenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Failed {
		// Track job failure
		if shouldAbort := m.handleJobFailure(logger, cluster, result.Name); shouldAbort {
			return m.triggerRollbackOrAbort(ctx, logger, cluster, "job failure threshold exceeded")
		}
		return true, nil // Requeue to retry with new job
	}
	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", ActionJoinGreenNonVoters)
		return true, nil
	}

	// Job succeeded - reset failure count
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""

	// All pods joined, transition to Syncing
	m.transitionToPhase(cluster, openbaov1alpha1.PhaseSyncing)

	return true, nil
}

// handlePhaseSyncing waits for Green nodes to catch up with Blue nodes.
func (m *Manager) handlePhaseSyncing(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	// Check MinSyncDuration if configured
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification.MinSyncDuration != "" {
		if cluster.Status.BlueGreen.StartTime == nil {
			return false, fmt.Errorf("StartTime is nil in Syncing phase")
		}

		minDuration, err := time.ParseDuration(cluster.Spec.UpdateStrategy.BlueGreen.Verification.MinSyncDuration)
		if err != nil {
			return false, fmt.Errorf("invalid MinSyncDuration: %w", err)
		}

		elapsed := time.Since(cluster.Status.BlueGreen.StartTime.Time)
		if elapsed < minDuration {
			logger.Info("Waiting for MinSyncDuration",
				"elapsed", elapsed,
				"minDuration", minDuration)
			return true, nil // Requeue to wait
		}
	}

	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionWaitGreenSynced,
		"",
		cluster.Status.BlueGreen.BlueRevision,
		cluster.Status.BlueGreen.GreenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Failed {
		// Track job failure
		if shouldAbort := m.handleJobFailure(logger, cluster, result.Name); shouldAbort {
			return m.triggerRollbackOrAbort(ctx, logger, cluster, "job failure threshold exceeded")
		}
		return true, nil // Requeue to retry
	}
	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", ActionWaitGreenSynced)
		return true, nil
	}

	// Job succeeded - reset failure count
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""

	// Check for pre-promotion hook
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.Verification.PrePromotionHook != nil {

		hook := cluster.Spec.UpdateStrategy.BlueGreen.Verification.PrePromotionHook
		hookResult, err := m.ensurePrePromotionHookJob(ctx, logger, cluster, hook)
		if err != nil {
			return false, fmt.Errorf("failed to ensure pre-promotion hook job: %w", err)
		}
		if hookResult.Running {
			logger.Info("Pre-promotion hook job is in progress", "job", hookResult.Name)
			return true, nil
		}
		if hookResult.Failed {
			logger.Info("Pre-promotion hook job failed", "job", hookResult.Name)
			// Check if auto-rollback on validation failure is enabled
			if cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback != nil &&
				cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback.OnValidationFailure {
				return m.triggerRollbackOrAbort(ctx, logger, cluster, "pre-promotion hook failed")
			}
			// Otherwise, stay in Syncing phase and wait for manual intervention
			return false, nil
		}
		logger.Info("Pre-promotion hook completed successfully", "job", hookResult.Name)
	}

	// Check if AutoPromote is disabled
	if cluster.Spec.UpdateStrategy.BlueGreen != nil && !cluster.Spec.UpdateStrategy.BlueGreen.AutoPromote {
		logger.Info("AutoPromote is disabled; waiting for manual approval")
		// Stay in Syncing phase until manual approval (annotation or field update)
		return false, nil
	}

	// All nodes synced, transition to Promoting
	m.transitionToPhase(cluster, openbaov1alpha1.PhasePromoting)

	return true, nil
}

// handlePhasePromoting promotes Green nodes to voters.
// In OpenBao's Raft, non-voters automatically become voters when they catch up,
// but we verify this and ensure all Green nodes are voters before proceeding.
func (m *Manager) handlePhasePromoting(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionPromoteGreenVoters,
		"",
		cluster.Status.BlueGreen.BlueRevision,
		cluster.Status.BlueGreen.GreenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Failed {
		// Track job failure
		if shouldAbort := m.handleJobFailure(logger, cluster, result.Name); shouldAbort {
			return m.triggerRollbackOrAbort(ctx, logger, cluster, "promotion job failure threshold exceeded")
		}
		return true, nil // Requeue to retry
	}
	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", ActionPromoteGreenVoters)
		return true, nil
	}

	// Job succeeded - reset failure count
	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""

	// Transition to TrafficSwitching (instead of DemotingBlue)
	// This decouples traffic switching from Blue demotion for safer rollbacks
	m.transitionToPhase(cluster, openbaov1alpha1.PhaseTrafficSwitching)

	return true, nil
}

// handlePhaseDemotingBlue demotes Blue nodes to non-voters and verifies Green becomes leader.
// After demotion, Blue nodes are no longer voters, so Green nodes (the only voters) will win any election.
// This phase includes the former "Cutover" logic - verifying Green is leader before proceeding.
func (m *Manager) handlePhaseDemotingBlue(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision

	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionDemoteBlueNonVotersStepDown,
		"",
		cluster.Status.BlueGreen.BlueRevision,
		cluster.Status.BlueGreen.GreenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Failed {
		return false, fmt.Errorf("upgrade executor Job %s failed for action %q", result.Name, ActionDemoteBlueNonVotersStepDown)
	}
	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", ActionDemoteBlueNonVotersStepDown)
		return true, nil
	}

	// After demotion, verify Green is now the leader (merged from former Cutover phase)
	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Green pods: %w", err)
	}

	var greenLeader *corev1.Pod
	for i := range greenPods {
		pod := &greenPods[i]
		active, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil {
			continue
		}
		if present && active {
			greenLeader = pod
			break
		}
	}

	if greenLeader == nil {
		logger.Info("Green leader not yet elected, waiting...")
		return true, nil // Requeue to wait for leader election
	}

	logger.Info("Green leader confirmed", "pod", greenLeader.Name)

	// Transition to Cleanup
	m.transitionToPhase(cluster, openbaov1alpha1.PhaseCleanup)

	return true, nil
}

// handlePhaseCleanup ejects Blue nodes from Raft and deletes the Blue StatefulSet.
// This is the "point of no return" - after this, rollback is not possible.
func (m *Manager) handlePhaseCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision

	// Step 1: Eject Blue nodes from Raft peer list
	// Get Green leader (should be the current leader after cutover)
	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Green pods: %w", err)
	}

	var greenLeader *corev1.Pod
	for i := range greenPods {
		pod := &greenPods[i]
		active, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil {
			continue
		}
		if present && active {
			greenLeader = pod
			break
		}
	}

	if greenLeader == nil {
		logger.Info("Green leader not found, waiting...")
		return true, nil // Requeue to wait for leader
	}

	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionRemoveBluePeers,
		"",
		blueRevision,
		greenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Failed {
		return false, fmt.Errorf("upgrade executor Job %s failed for action %q", result.Name, ActionRemoveBluePeers)
	}
	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", ActionRemoveBluePeers)
		return true, nil
	}

	// Step 2: Delete Blue StatefulSet
	blueStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, blueRevision)
	blueStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      blueStatefulSetName,
	}, blueStatefulSet); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get Blue StatefulSet: %w", err)
		}
		// Already deleted
		logger.Info("Blue StatefulSet already deleted", "blueRevision", blueRevision)
	} else {
		// Delete the StatefulSet - this will cascade-delete its pods
		if err := m.client.Delete(ctx, blueStatefulSet); err != nil {
			return false, fmt.Errorf("failed to delete Blue StatefulSet: %w", err)
		}
		logger.Info("Deleted Blue StatefulSet", "blueRevision", blueRevision)
		return true, nil // Requeue to verify deletion and wait for pods to terminate
	}

	// Verify Blue pods are gone (excluding Terminating pods)
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return false, fmt.Errorf("failed to check Blue pods: %w", err)
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
		return true, nil // Requeue to wait
	}

	// Finalize upgrade
	cluster.Status.CurrentVersion = cluster.Spec.Version
	cluster.Status.BlueGreen.BlueRevision = cluster.Status.BlueGreen.GreenRevision
	cluster.Status.BlueGreen.GreenRevision = ""
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
	cluster.Status.BlueGreen.StartTime = nil

	// Update conditions
	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeComplete,
		Message:            fmt.Sprintf("Blue/green upgrade to %s completed", cluster.Spec.Version),
	})

	logger.Info("Blue/green upgrade completed", "newVersion", cluster.Spec.Version)

	return false, nil
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

	// Set degraded condition
	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeFailed,
		Message:            "Blue/green upgrade aborted due to Green cluster failure",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeFailed,
		Message:            "Blue/green upgrade aborted",
	})

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

// handleJobFailure increments the job failure count and returns true if threshold exceeded.
func (m *Manager) handleJobFailure(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) bool {
	cluster.Status.BlueGreen.JobFailureCount++
	cluster.Status.BlueGreen.LastJobFailure = jobName

	maxFailures := m.getMaxJobFailures(cluster)
	logger.Info("Job failure recorded",
		"job", jobName,
		"failureCount", cluster.Status.BlueGreen.JobFailureCount,
		"maxFailures", maxFailures)

	return cluster.Status.BlueGreen.JobFailureCount >= maxFailures
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
func (m *Manager) triggerRollbackOrAbort(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, reason string) (bool, error) {
	phase := cluster.Status.BlueGreen.Phase

	if isEarlyPhase(phase) {
		// Early phase: simple abort (delete Green, reset to Idle)
		logger.Info("Aborting upgrade due to failures in early phase", "phase", phase, "reason", reason)
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return false, fmt.Errorf("failed to abort upgrade: %w", err)
		}
		return false, nil
	}

	// Late phase: full rollback required
	logger.Info("Triggering rollback due to failures in late phase", "phase", phase, "reason", reason)
	return m.triggerRollback(logger, cluster, reason)
}

// triggerRollback initiates rollback from any phase.
func (m *Manager) triggerRollback(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, reason string) (bool, error) {
	now := metav1.Now()
	cluster.Status.BlueGreen.RollbackReason = reason
	cluster.Status.BlueGreen.RollbackStartTime = &now
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollingBack

	logger.Info("Rollback initiated", "reason", reason)

	// Set condition
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "UpgradeRollback",
		Message:            fmt.Sprintf("Blue/green upgrade rolling back: %s", reason),
	})

	return true, nil // Requeue to process rollback
}

// handlePhaseTrafficSwitching handles the TrafficSwitching phase.
// This phase switches traffic to Green and optionally waits for stabilization.
func (m *Manager) handlePhaseTrafficSwitching(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision

	// Check if traffic has already been switched (by checking if TrafficSwitchedTime is set)
	if cluster.Status.BlueGreen.TrafficSwitchedTime == nil {
		// Traffic not yet switched - switch now
		// The actual Service selector update is handled by infra.Manager based on the phase
		now := metav1.Now()
		cluster.Status.BlueGreen.TrafficSwitchedTime = &now
		logger.Info("Traffic switched to Green", "greenRevision", greenRevision)
		return true, nil // Requeue to start stabilization
	}

	// Check stabilization period
	stabilizationSeconds := int32(60) // Default
	if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback != nil &&
		cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback.StabilizationSeconds != nil {
		stabilizationSeconds = *cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback.StabilizationSeconds
	}

	elapsed := time.Since(cluster.Status.BlueGreen.TrafficSwitchedTime.Time)
	requiredDuration := time.Duration(stabilizationSeconds) * time.Second

	if elapsed < requiredDuration {
		logger.Info("Waiting for stabilization period",
			"elapsed", elapsed,
			"required", requiredDuration)

		// Check Green health during stabilization if OnTrafficFailure is enabled
		if cluster.Spec.UpdateStrategy.BlueGreen != nil &&
			cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback != nil &&
			cluster.Spec.UpdateStrategy.BlueGreen.AutoRollback.OnTrafficFailure {
			greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
			if err != nil {
				return false, fmt.Errorf("failed to get Green pods: %w", err)
			}

			// Check for unhealthy pods
			for _, pod := range greenPods {
				if !isPodReady(&pod) {
					logger.Info("Green pod unhealthy during stabilization, triggering rollback", "pod", pod.Name)
					return m.triggerRollback(logger, cluster, "Green pod became unhealthy during stabilization")
				}
			}
		}

		return true, nil // Requeue to wait
	}

	// Stabilization complete, proceed to DemotingBlue
	logger.Info("Stabilization period complete, proceeding to demote Blue")
	m.transitionToPhase(cluster, openbaov1alpha1.PhaseDemotingBlue)

	return true, nil
}

// handlePhaseRollingBack orchestrates the rollback sequence.
func (m *Manager) handlePhaseRollingBack(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	blueRevision := cluster.Status.BlueGreen.BlueRevision
	greenRevision := cluster.Status.BlueGreen.GreenRevision

	// Step 1: Re-promote Blue nodes to voters (if they were demoted)
	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionPromoteBlueVoters, // New action
		"rollback",
		blueRevision,
		greenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: promoting Blue voters", "job", result.Name)
		return true, nil
	}
	if result.Failed {
		// Rollback failed - this is serious, set critical condition
		logger.Error(nil, "Rollback job failed", "job", result.Name)
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             "RollbackFailed",
			Message:            fmt.Sprintf("Rollback job %s failed - manual intervention required", result.Name),
		})
		return false, nil // Don't requeue, needs manual intervention
	}

	// Step 2: Demote Green nodes to non-voters
	result, err = EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ActionDemoteGreenNonVoters, // New action
		"rollback",
		blueRevision,
		greenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: demoting Green non-voters", "job", result.Name)
		return true, nil
	}
	if result.Failed {
		logger.Error(nil, "Rollback demote job failed", "job", result.Name)
		return false, nil // Needs manual intervention
	}

	// Step 3: Verify Blue leader is elected
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Blue pods: %w", err)
	}

	var blueLeader *corev1.Pod
	for i := range bluePods {
		pod := &bluePods[i]
		active, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil {
			continue
		}
		if present && active {
			blueLeader = pod
			break
		}
	}

	if blueLeader == nil {
		logger.Info("Blue leader not yet elected during rollback, waiting...")
		return true, nil // Requeue to wait for leader election
	}

	logger.Info("Blue leader confirmed during rollback", "pod", blueLeader.Name)

	// Transition to RollbackCleanup
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollbackCleanup
	now := metav1.Now()
	cluster.Status.BlueGreen.StartTime = &now

	return true, nil
}

// handlePhaseRollbackCleanup removes Green StatefulSet after rollback.
func (m *Manager) handlePhaseRollbackCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
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
		ActionRemoveGreenPeers, // New action
		"rollback",
		blueRevision,
		greenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Running {
		logger.Info("Rollback job in progress: removing Green peers", "job", result.Name)
		return true, nil
	}
	// Continue even if job failed - we still want to delete the StatefulSet

	// Step 2: Delete Green StatefulSet
	if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
		return false, fmt.Errorf("failed to cleanup Green StatefulSet during rollback: %w", err)
	}

	// Step 3: Verify Green pods are gone
	greenPods, err := m.getGreenPods(ctx, cluster, greenRevision)
	if err != nil {
		return false, fmt.Errorf("failed to check Green pods: %w", err)
	}

	activeGreenPods := 0
	for _, pod := range greenPods {
		if pod.DeletionTimestamp == nil {
			activeGreenPods++
		}
	}

	if activeGreenPods > 0 {
		logger.Info("Green pods still exist during rollback cleanup, waiting", "count", activeGreenPods)
		return true, nil // Requeue to wait
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

	// Update conditions
	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "UpgradeRolledBack",
		Message:            fmt.Sprintf("Blue/green upgrade rolled back: %s", rollbackReason),
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "RollbackComplete",
		Message:            "Rollback completed, cluster restored to previous version",
	})

	logger.Info("Blue/green rollback completed", "reason", rollbackReason)

	return false, nil
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

// PrePromotionHookResult represents the result of checking a pre-promotion hook job.
type PrePromotionHookResult struct {
	Name    string
	Running bool
	Failed  bool
}

// ensurePrePromotionHookJob creates or checks the status of a pre-promotion validation job.
func (m *Manager) ensurePrePromotionHookJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	hook *openbaov1alpha1.ValidationHookConfig,
) (*PrePromotionHookResult, error) {
	jobName := fmt.Sprintf("%s-validation-hook", cluster.Name)

	// Check if job exists
	existingJob := &batchv1.Job{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, existingJob)

	if err == nil {
		// Job exists, check status
		if existingJob.Status.Succeeded > 0 {
			return &PrePromotionHookResult{Name: jobName, Running: false, Failed: false}, nil
		}
		if existingJob.Status.Failed > 0 {
			return &PrePromotionHookResult{Name: jobName, Running: false, Failed: true}, nil
		}
		return &PrePromotionHookResult{Name: jobName, Running: true, Failed: false}, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get job %s: %w", jobName, err)
	}

	// Create the validation job
	timeout := int32(300)
	if hook.TimeoutSeconds != nil {
		timeout = *hook.TimeoutSeconds
	}
	backoffLimit := int32(0) // No retries
	ttlSeconds := int32(3600)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: "validation-hook",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   ptr.To(int64(timeout)),
			TTLSecondsAfterFinished: &ttlSeconds,
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
	}

	if err := m.client.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create validation hook job: %w", err)
	}

	logger.Info("Created pre-promotion validation hook job", "job", jobName)
	return &PrePromotionHookResult{Name: jobName, Running: true, Failed: false}, nil
}

// createPreUpgradeSnapshotJob creates a backup job at the start of an upgrade.
// This is called when the upgrade starts to ensure we have a recovery point.
func (m *Manager) createPreUpgradeSnapshotJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (string, error) {
	if cluster.Spec.Backup == nil || cluster.Spec.Backup.Target.Endpoint == "" {
		return "", fmt.Errorf("backup configuration required for pre-upgrade snapshot")
	}

	if cluster.Spec.Backup.ExecutorImage == "" {
		return "", fmt.Errorf("backup executor image is required for snapshots")
	}

	// Generate a unique job name
	jobName := fmt.Sprintf("%s-preupgrade-snapshot-%d", cluster.Name, time.Now().Unix())

	// Build the snapshot job
	job, err := m.buildSnapshotJob(cluster, jobName, "pre-upgrade")
	if err != nil {
		return "", fmt.Errorf("failed to build snapshot job: %w", err)
	}

	// Create the job
	if err := m.client.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Info("Snapshot job already exists", "job", jobName)
			return jobName, nil
		}
		return "", fmt.Errorf("failed to create snapshot job: %w", err)
	}

	logger.Info("Created pre-upgrade snapshot job", "job", jobName)
	return jobName, nil
}

// buildSnapshotJob creates a backup Job spec for upgrade snapshots.
func (m *Manager) buildSnapshotJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName, phase string) (*batchv1.Job, error) {
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

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: "upgrade-snapshot",
			},
			Annotations: map[string]string{
				"openbao.org/snapshot-phase": phase,
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
						constants.LabelOpenBaoComponent: "upgrade-snapshot",
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
							Image: cluster.Spec.Backup.ExecutorImage,
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

	return job, nil
}
