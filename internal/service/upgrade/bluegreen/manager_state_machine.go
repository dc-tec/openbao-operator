package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

// calculateRevision computes a deterministic revision hash from relevant spec fields.
func (m *Manager) calculateRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
}

// executeStateMachine runs the blue/green upgrade state machine.
func (m *Manager) executeStateMachine(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string, acknowledgements *upgrade.RequestAcknowledgements) (recon.Result, error) {
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
		openbaov1alpha1.PhaseJoiningMesh:           m.handlePhaseJoiningMesh,
		openbaov1alpha1.PhaseSyncing:               m.handlePhaseSyncing,
		openbaov1alpha1.PhasePromoting:             m.handlePhasePromoting,
		openbaov1alpha1.PhaseDemotingBlue:          m.handlePhaseDemotingBlue,
		openbaov1alpha1.PhaseCleanup:               m.handlePhaseCleanup,
		openbaov1alpha1.PhaseRestoringReadReplicas: m.handlePhaseRestoringReadReplicas,
		openbaov1alpha1.PhaseRollingBack:           m.handlePhaseRollingBack,
		openbaov1alpha1.PhaseRollbackCleanup:       m.handlePhaseRollbackCleanup,
	}

	handler, ok := handlers[phase]
	if !ok {
		return recon.Result{}, fmt.Errorf("unknown blue/green phase: %s", phase)
	}

	outcome, err := handler(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	result, err := m.applyOutcome(ctx, logger, cluster, outcome)
	if err == nil {
		acknowledgements.Merge(outcome.acknowledgements)
	}
	return result, err
}

func (m *Manager) applyOutcome(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, outcome phaseOutcome) (recon.Result, error) {
	if err := outcome.validate(); err != nil {
		return recon.Result{}, err
	}

	switch outcome.kind {
	case phaseOutcomeAdvance:
		previousPhase := cluster.Status.BlueGreen.Phase
		core.AdvanceBlueGreenPhase(cluster.Status.BlueGreen, outcome.nextPhase)
		opslifecycle.LogPhaseTransition(logger, logging.EventBlueGreenPhaseTransition, string(previousPhase), string(outcome.nextPhase), map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
		})
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
