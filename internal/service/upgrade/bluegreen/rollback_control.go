package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// checkAbortConditions checks if the upgrade should be aborted due to Green cluster failures.
func (m *Manager) checkAbortConditions(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle {
		return false, nil
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	if greenRevision == "" {
		return false, nil
	}

	greenPods, err := m.getPodsByRevision(ctx, cluster, greenRevision)
	if err != nil {
		return false, fmt.Errorf("failed to get Green pods: %w", err)
	}

	for _, pod := range greenPods {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Waiting != nil {
				reason := containerStatus.State.Waiting.Reason
				if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					logger.Info("Green pod in failure state, aborting upgrade", "pod", pod.Name, "reason", reason)
					return true, nil
				}
			}
			if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
				logger.Info("Green pod terminated with error, aborting upgrade", "pod", pod.Name, "exitCode", containerStatus.State.Terminated.ExitCode)
				return true, nil
			}
		}
	}

	return false, nil
}

// getMaxJobFailures returns the configured max job failures threshold or default (5).
func (m *Manager) getMaxJobFailures(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if cluster.Spec.Upgrade.BlueGreen != nil && cluster.Spec.Upgrade.BlueGreen.MaxJobFailures != nil {
		return *cluster.Spec.Upgrade.BlueGreen.MaxJobFailures
	}
	return 5
}

// isEarlyPhase returns true if the upgrade is in an early phase where abort is appropriate.
func isEarlyPhase(phase openbaov1alpha1.BlueGreenPhase) bool {
	switch phase {
	case openbaov1alpha1.PhaseDeployingGreen, openbaov1alpha1.PhaseJoiningMesh, openbaov1alpha1.PhaseSyncing:
		return true
	default:
		return false
	}
}

// triggerRollbackOrAbort decides whether to abort early phases or trigger a full rollback in later phases.
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
		logger.Info("Aborting upgrade due to failures in early phase", "phase", phase, "reason", reason)
		if err := m.abortUpgrade(ctx, logger, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to abort upgrade: %w", err)
		}
		return recon.Result{}, nil
	}

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

	return requeueShort(), nil
}
