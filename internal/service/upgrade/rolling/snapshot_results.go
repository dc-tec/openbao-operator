package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	snapshothelpers "github.com/dc-tec/openbao-operator/internal/service/upgrade/snapshot"
)

func (m *Manager) reconcileExistingPreUpgradeBackupJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	jobState snapshothelpers.JobState,
) (bool, error) {
	return snapshothelpers.ReconcileExistingJob(logger, jobName, jobState, snapshothelpers.ExistingJobHandlers{
		OnFound: func(jobName string) {
			logger.Info("Found existing pre-upgrade backup job, checking status", "job", jobName)
		},
		OnRunning: func(jobName string) {
			logger.Info("Pre-upgrade backup job is still running", "job", jobName)
		},
		OnFailed: func(jobName string) (bool, error) {
			return m.handleFailedPreUpgradeBackupJob(ctx, logger, cluster, jobName)
		},
		OnSucceeded: func(jobName string) (bool, error) {
			m.recordSuccessfulPreUpgradeBackupJob(logger, cluster, jobName)
			return true, nil
		},
	})
}

func (m *Manager) handleFailedPreUpgradeBackupJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (bool, error) {
	expectedJobName := m.backupJobName(cluster)
	failedCount, err := m.countFailedPreUpgradeBackupJobs(ctx, cluster, expectedJobName)
	if err != nil {
		return false, fmt.Errorf("failed to count failed backup jobs: %w", err)
	}

	maxRetries := upgrade.DefaultMaxPreUpgradeBackupRetries
	if failedCount >= maxRetries {
		logging.LogAuditEvent(logger, logging.EventPreUpgradeSnapshotFailed, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"job":               jobName,
			"attempts":          fmt.Sprintf("%d", failedCount),
		})
		m.emitWarningEvent(cluster, upgrade.ReasonPreUpgradeSnapshotFailed, "Pre-upgrade snapshot failed after %d attempts; last Job was %s", failedCount, jobName)
		return false, operatorerrors.WithReason(
			upgrade.ReasonPreUpgradeBackupFailed,
			fmt.Errorf("pre-upgrade backup failed after %d attempts (max retries exceeded); manual intervention required", failedCount),
		)
	}

	logger.Info("Deleting failed pre-upgrade backup job for retry",
		"job", jobName,
		"attempt", failedCount+1,
		"maxRetries", maxRetries)
	if err := m.deletePreUpgradeBackupJob(ctx, jobName, cluster.Namespace); err != nil {
		return false, fmt.Errorf("failed to delete failed backup job %s: %w", jobName, err)
	}

	logging.LogAuditEvent(logger, logging.EventPreUpgradeSnapshotRetry, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"job":               jobName,
		"attempt":           fmt.Sprintf("%d", failedCount+1),
		"max_retries":       fmt.Sprintf("%d", maxRetries),
	})
	return false, nil
}

func (m *Manager) recordSuccessfulPreUpgradeBackupJob(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) {
	logger.Info("Pre-upgrade backup job completed successfully", "job", jobName)
	logging.LogAuditEvent(logger, logging.EventPreUpgradeSnapshotCompleted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"job":               jobName,
	})
	if cluster.Status.Upgrade == nil {
		m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotCompleted, "Pre-upgrade snapshot completed successfully with Job %s", jobName)
	}
}
