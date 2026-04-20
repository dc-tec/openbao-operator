package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	configurationservice "github.com/dc-tec/openbao-operator/internal/service/configuration"
)

// handlePhaseIdle transitions from Idle to DeployingGreen when an upgrade is detected.
func (m *Manager) handlePhaseIdle(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	m.recordBlueGreenUpgradeStart(logger, cluster)

	if outcome, waiting, err := m.ensureBlueGreenPreUpgradeSnapshotComplete(ctx, logger, cluster); waiting || err != nil {
		return outcome, err
	}

	cluster.Status.BlueGreen.GreenRevision = m.calculateRevision(cluster)
	return advance(openbaov1alpha1.PhaseDeployingGreen), nil
}

// handlePhaseDeployingGreen creates the Green StatefulSet.
// IMPORTANT: Green pods must join the existing Blue cluster as non-voters, not initialize a new cluster.
func (m *Manager) handlePhaseDeployingGreen(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, _ string) (phaseOutcome, error) {
	greenRevision := cluster.Status.BlueGreen.GreenRevision
	blueRevision := cluster.Status.BlueGreen.BlueRevision
	logger = logger.WithValues("greenRevision", greenRevision, "blueRevision", blueRevision)

	if outcome, waiting, err := m.ensureBlueClusterReadyForGreen(ctx, logger, cluster, blueRevision); waiting || err != nil {
		return outcome, err
	}
	if outcome, waiting, err := m.ensureGreenStatefulSetReady(ctx, logger, cluster, blueRevision, greenRevision); waiting || err != nil {
		return outcome, err
	}

	return advance(openbaov1alpha1.PhaseJoiningMesh), nil
}

func (m *Manager) createGreenStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, blueRevision string, greenRevision string) (phaseOutcome, error) {
	if m.workloadRuntime == nil {
		return phaseOutcome{}, fmt.Errorf("workload runtime is not configured")
	}

	configContent, err := configurationservice.Render(cluster, configurationservice.RenderOptions{
		TargetRevisionForJoin: blueRevision,
	})
	if err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to render config for Green cluster: %w", err)
	}

	greenImage, greenInitImage, err := m.prepareGreenStatefulSetImages(ctx, logger, cluster)
	if err != nil {
		return phaseOutcome{}, err
	}

	if err := m.workloadRuntime.EnsureStatefulSetWithRevision(ctx, logger, cluster, configContent, greenImage, greenInitImage, greenRevision, true); err != nil {
		return phaseOutcome{}, fmt.Errorf("failed to create Green StatefulSet: %w", err)
	}

	logger.Info("Created Green StatefulSet", "greenRevision", greenRevision)
	return requeueAfterOutcome(constants.RequeueShort), nil
}

func (m *Manager) prepareGreenStatefulSetImages(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, string, error) {
	greenImage := cluster.Spec.Image
	verifiedGreenDigest, err := m.verifyImageDigest(ctx, logger, cluster, greenImage, constants.ReasonBlueGreenImageVerificationFailed, "Green image verification failed")
	if err != nil {
		return "", "", err
	}
	if verifiedGreenDigest != "" {
		greenImage = verifiedGreenDigest
	}

	initImage, err := resolveInitContainerImage(cluster)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve Green init container image: %w", err)
	}

	verifiedInitContainerDigest, err := m.verifyOperatorImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Green init container image verification failed")
	if err != nil {
		return "", "", err
	}
	if verifiedInitContainerDigest != "" {
		initImage = verifiedInitContainerDigest
	}

	return greenImage, initImage, nil
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

	return advance(openbaov1alpha1.PhaseSyncing), nil
}
