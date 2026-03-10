package bluegreen

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// handlePhaseIdle transitions from Idle to DeployingGreen when an upgrade is detected.
func (m *Manager) handlePhaseIdle(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	logger.Info("Starting blue/green upgrade",
		"fromVersion", cluster.Status.CurrentVersion,
		"targetVersion", cluster.Spec.Version)
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.PreUpgradeSnapshotJobName == "" {
		m.emitNormalEvent(cluster, ReasonUpgradeStarted, "Blue/green upgrade started from %s to %s", cluster.Status.CurrentVersion, cluster.Spec.Version)
	}
	logging.LogAuditEvent(logger, logging.EventUpgradeStarted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"strategy":          string(openbaov1alpha1.UpdateStrategyBlueGreen),
		"from_version":      cluster.Status.CurrentVersion,
		"to_version":        cluster.Spec.Version,
	})

	if cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.GreenRevision == "" &&
		cluster.Status.BlueGreen.PreUpgradeSnapshotJobName == "" {
		cluster.Status.BlueGreen.ManualPromotionRequired = cluster.Spec.Upgrade.BlueGreen != nil &&
			!cluster.Spec.Upgrade.BlueGreen.AutoPromote
	}

	// Pre-upgrade snapshot (if enabled)
	preUpgradeSnapshotEnabled := cluster.Spec.Upgrade.PreUpgradeSnapshot ||
		(cluster.Spec.Upgrade.BlueGreen != nil && cluster.Spec.Upgrade.BlueGreen.PreUpgradeSnapshot)
	if preUpgradeSnapshotEnabled {
		jobName := preUpgradeSnapshotJobName(cluster)
		if cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != jobName {
			_, err := m.ensurePreUpgradeSnapshotJob(ctx, logger, cluster, jobName)
			if err != nil {
				logger.Error(err, "Failed to ensure pre-upgrade snapshot job")
				return phaseOutcome{}, err // Block upgrade on snapshot failure
			}
			cluster.Status.BlueGreen.PreUpgradeSnapshotJobName = jobName
			m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotJobCreated, "Created pre-upgrade snapshot Job %s", jobName)
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
			} else if jobStatus.Exists && jobStatus.Failed {
				m.emitWarningEvent(cluster, upgrade.ReasonPreUpgradeSnapshotFailed, "Pre-upgrade snapshot Job %s failed", jobName)
				logger.Info("Pre-upgrade snapshot failed", "job", jobName)
				return phaseOutcome{}, fmt.Errorf("pre-upgrade snapshot job failed: %s", jobName) // Block
			}
			m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotCompleted, "Pre-upgrade snapshot completed successfully with Job %s", jobName)
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
			sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
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

		if m.infraRuntime == nil {
			return phaseOutcome{}, fmt.Errorf("infra runtime is not configured")
		}
		if err := m.infraRuntime.EnsureStatefulSetWithRevision(ctx, logger, cluster, configContent, imageForGreen, verifiedInitContainerDigest, greenRevision, true); err != nil {
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
		sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
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

	// Check if this in-flight upgrade requires an explicit promote request.
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

	// All nodes synced, transition to Promoting
	m.emitNormalEvent(cluster, ReasonBlueGreenPromotionApproved, "Promotion approved for Green revision %s", cluster.Status.BlueGreen.GreenRevision)
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

	return advance(openbaov1alpha1.PhaseDemotingBlue), nil
}

// handlePhaseDemotingBlue demotes Blue nodes to non-voters and verifies Green becomes leader.
// After demotion, Blue nodes are no longer voters, so Green nodes (the only voters) will win any election.
// This phase includes the former "Cutover" logic - verifying Green is leader before proceeding.
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

	ok, message := demotionPreconditionsSatisfied(
		greenSnapshots,
		int(cluster.Spec.Replicas),
	)
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

	// After demotion, verify Green is now the leader (merged from former Cutover phase)
	leaderPod, source, ok := m.clusterOps.FindLeaderPod(ctx, logger, cluster, greenPods)
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

	// Finalize upgrade.
	// NOTE: CurrentVersion is updated by the Status controller when it detects
	// BlueGreen.Phase == Idle and version mismatch. This maintains clean SSA field ownership.
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

	// Return a requeue to trigger another reconcile cycle so dependent reconcilers
	// can observe the new steady-state and clean up any upgrade-time resources.
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

	if greenRevision != "" {
		// Delete Green StatefulSet
		if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
			return fmt.Errorf("failed to cleanup Green StatefulSet during abort: %w", err)
		}
	}

	if err := m.finalizeUpgradeTerminalState(ctx, logger, cluster, false); err != nil {
		return err
	}

	logger.Info("Blue/green upgrade aborted successfully")

	return nil
}

// getMaxJobFailures returns the configured max job failures threshold or default (5).
func (m *Manager) getMaxJobFailures(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if cluster.Spec.Upgrade.BlueGreen != nil &&
		cluster.Spec.Upgrade.BlueGreen.MaxJobFailures != nil {
		return *cluster.Spec.Upgrade.BlueGreen.MaxJobFailures
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
	logging.LogAuditEvent(logger, logging.EventUpgradeFailed, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"strategy":          string(openbaov1alpha1.UpdateStrategyBlueGreen),
		"reason":            reason,
	})
	m.emitWarningEvent(cluster, ReasonUpgradeFailed, "Blue/green upgrade failed: %s", reason)

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
	logging.LogAuditEvent(logger, logging.EventRollbackInitiated, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"reason":            reason,
	})
	m.emitWarningEvent(cluster, ReasonRollbackStarted, "Blue/green rollback started: %s", reason)

	return requeueShort(), nil // Requeue to process rollback
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
		m.clientConfig,
		m.operatorImageVerifier,
		m.Platform,
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
		m.enterBreakGlassRollbackConsensusRepairFailed(logger, cluster, result.Name)
		return hold(), nil
	}

	// Step 3: Verify Blue leader is elected
	bluePods, err := m.getBluePods(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to get Blue pods: %w", err)
	}

	leaderPod, source, ok := m.clusterOps.FindLeaderPod(ctx, logger, cluster, bluePods)
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
		ActionRemoveGreenPeers,
		rollbackRunID(cluster),
		blueRevision,
		greenRevision,
		m.clientConfig,
		m.operatorImageVerifier,
		m.Platform,
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
	// Keep RollbackReason and RollbackStartTime for observability
	if err := m.finalizeUpgradeTerminalState(ctx, logger, cluster, false); err != nil {
		return phaseOutcome{}, err
	}

	logger.Info("Blue/green rollback completed", "reason", rollbackReason)
	logging.LogAuditEvent(logger, logging.EventRollbackCompleted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"reason":            rollbackReason,
	})

	return done(), nil
}
