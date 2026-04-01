package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
	snapshothelpers "github.com/dc-tec/openbao-operator/internal/service/upgrade/snapshot"
)

func (m *Manager) recordBlueGreenUpgradeStart(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	logger.Info("Starting blue/green upgrade",
		"fromVersion", cluster.Status.CurrentVersion,
		"targetVersion", cluster.Spec.Version)
	if core.BlueGreenStartEventPending(cluster) {
		m.emitNormalEvent(cluster, ReasonUpgradeStarted, "Blue/green upgrade started from %s to %s", cluster.Status.CurrentVersion, cluster.Spec.Version)
	}
	logging.LogAuditEvent(logger, logging.EventUpgradeStarted, upgrade.UpgradeStartedAuditFields(
		cluster,
		string(openbaov1alpha1.UpdateStrategyBlueGreen),
		cluster.Status.CurrentVersion,
		cluster.Spec.Version,
	))

	core.InitializeBlueGreenManualPromotion(cluster)
}

func blueGreenPreUpgradeSnapshotEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster.Spec.Upgrade.PreUpgradeSnapshot ||
		(cluster.Spec.Upgrade.BlueGreen != nil && cluster.Spec.Upgrade.BlueGreen.PreUpgradeSnapshot)
}

func (m *Manager) ensureBlueGreenPreUpgradeSnapshotComplete(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, bool, error) {
	if !blueGreenPreUpgradeSnapshotEnabled(cluster) {
		return phaseOutcome{}, false, nil
	}
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, true, fmt.Errorf("blue/green status is nil")
	}

	jobName := preUpgradeSnapshotJobName(cluster)
	if cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != jobName {
		_, err := m.ensurePreUpgradeSnapshotJob(ctx, logger, cluster, jobName)
		if err != nil {
			logger.Error(err, "Failed to ensure pre-upgrade snapshot job")
			return phaseOutcome{}, true, err
		}
		cluster.Status.BlueGreen.PreUpgradeSnapshotJobName = jobName
		m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotJobCreated, "Created pre-upgrade snapshot Job %s", jobName)
		logger.Info("Pre-upgrade snapshot job created", "job", jobName)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}

	jobStatus, err := getJobStatus(ctx, m.client, cluster, jobName)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("failed to check pre-upgrade snapshot job status: %w", err)
	}
	complete, err := snapshothelpers.ReconcileExistingJob(logger, jobName, snapshothelpers.JobStateFromResult(jobStatus), snapshothelpers.ExistingJobHandlers{
		OnFound: func(string) {},
		OnRunning: func(jobName string) {
			logger.Info("Waiting for pre-upgrade snapshot to complete", "job", jobName)
		},
		OnFailed: func(jobName string) (bool, error) {
			m.emitWarningEvent(cluster, upgrade.ReasonPreUpgradeSnapshotFailed, "Pre-upgrade snapshot Job %s failed", jobName)
			logger.Info("Pre-upgrade snapshot failed", "job", jobName)
			return false, fmt.Errorf("pre-upgrade snapshot job failed: %s", jobName)
		},
		OnSucceeded: func(jobName string) (bool, error) {
			m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotCompleted, "Pre-upgrade snapshot completed successfully with Job %s", jobName)
			logger.Info("Pre-upgrade snapshot completed", "job", jobName)
			return true, nil
		},
	})
	if err != nil {
		return phaseOutcome{}, true, err
	}
	if !complete {
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	return phaseOutcome{}, false, nil
}

// abortUpgrade aborts the blue/green upgrade by cleaning up Green resources.
func (m *Manager) abortUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.BlueGreen == nil {
		return nil
	}

	greenRevision := cluster.Status.BlueGreen.GreenRevision
	if greenRevision == "" {
		return nil
	}

	logger.Info("Aborting blue/green upgrade", "greenRevision", greenRevision)
	if err := m.cleanupGreenStatefulSet(ctx, logger, cluster); err != nil {
		return fmt.Errorf("failed to cleanup Green StatefulSet during abort: %w", err)
	}
	if err := m.finalizeUpgradeTerminalState(ctx, logger, cluster, false); err != nil {
		return err
	}

	logger.Info("Blue/green upgrade aborted successfully")
	return nil
}

func (m *Manager) completeBlueGreenUpgrade(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, error) {
	if err := m.finalizeUpgradeTerminalState(ctx, logger, cluster, true); err != nil {
		logger.Error(err, "Failed to finalize blue/green terminal state")
		return phaseOutcome{}, err
	}

	logger.Info("Blue/green upgrade completed", "newVersion", cluster.Spec.Version)
	logging.LogAuditEvent(logger, logging.EventUpgradeCompleted, upgrade.UpgradeCompletedAuditFields(
		cluster,
		string(openbaov1alpha1.UpdateStrategyBlueGreen),
		cluster.Spec.Version,
	))
	m.emitNormalEvent(cluster, ReasonUpgradeComplete, "Blue/green upgrade completed for target version %s", cluster.Spec.Version)

	return requeueAfterOutcome(constants.RequeueShort), nil
}

func (m *Manager) completeBlueGreenRollback(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	rollbackReason string,
) (phaseOutcome, error) {
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
