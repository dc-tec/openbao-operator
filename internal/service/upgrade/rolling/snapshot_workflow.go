package rolling

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

type preUpgradeBackupJobState string

const (
	preUpgradeBackupJobStateNone      preUpgradeBackupJobState = ""
	preUpgradeBackupJobStateRunning   preUpgradeBackupJobState = "running"
	preUpgradeBackupJobStateFailed    preUpgradeBackupJobState = "failed"
	preUpgradeBackupJobStateSucceeded preUpgradeBackupJobState = "succeeded"
)

// handlePreUpgradeSnapshot checks if preUpgradeSnapshot is enabled and triggers a backup if needed.
// Returns true if the snapshot is complete (or disabled), false if it is in progress (created or running).
// Returns an error if backup fails, which will block the upgrade.
func (m *Manager) handlePreUpgradeSnapshot(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Spec.Upgrade == nil || !cluster.Spec.Upgrade.PreUpgradeSnapshot {
		logger.V(1).Info("Pre-upgrade snapshot is not enabled")
		return true, nil
	}

	if err := m.validatePreUpgradeSnapshotPrerequisites(ctx, cluster); err != nil {
		return false, err
	}

	existingJobName, existingJobStatus, err := m.findExistingPreUpgradeBackupJob(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to check for existing pre-upgrade backup job: %w", err)
	}
	if existingJobName != "" {
		return m.reconcileExistingPreUpgradeBackupJob(ctx, logger, cluster, existingJobName, existingJobStatus)
	}

	return m.createPreUpgradeBackupJob(ctx, logger, cluster)
}

func (m *Manager) reconcileExistingPreUpgradeBackupJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	jobState preUpgradeBackupJobState,
) (bool, error) {
	logger.Info("Found existing pre-upgrade backup job, checking status", "job", jobName)

	switch jobState {
	case preUpgradeBackupJobStateFailed:
		return m.handleFailedPreUpgradeBackupJob(ctx, logger, cluster, jobName)
	case preUpgradeBackupJobStateSucceeded:
		m.recordSuccessfulPreUpgradeBackupJob(logger, cluster, jobName)
		return true, nil
	default:
		logger.Info("Pre-upgrade backup job is still running", "job", jobName)
		return false, nil
	}
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

func (m *Manager) createPreUpgradeBackupJob(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobName := m.backupJobName(cluster)
	logger.Info("Creating pre-upgrade backup job", "job", jobName)

	if err := m.ensurePreUpgradeBackupRuntime(ctx, cluster); err != nil {
		return false, err
	}

	verifiedExecutorDigest, err := m.resolvePreUpgradeBackupExecutorDigest(ctx, logger, cluster)
	if err != nil {
		return false, err
	}

	job, err := m.buildPreUpgradeBackupJob(cluster, jobName, verifiedExecutorDigest)
	if err != nil {
		return false, err
	}

	if err := m.client.Create(ctx, job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.V(1).Info("Pre-upgrade backup job already exists after create attempt", "job", jobName)
			return false, nil
		}
		return false, fmt.Errorf("failed to create backup job: %w", err)
	}

	logger.Info("Pre-upgrade backup job created", "job", jobName)
	logging.LogAuditEvent(logger, logging.EventPreUpgradeSnapshotJobCreated, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"job":               jobName,
	})
	m.emitNormalEvent(cluster, upgrade.ReasonPreUpgradeSnapshotJobCreated, "Created pre-upgrade snapshot Job %s", jobName)
	return false, nil
}

func (m *Manager) ensurePreUpgradeBackupRuntime(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if m.backupRuntime == nil {
		return fmt.Errorf("backup runtime is not configured")
	}
	if err := m.backupRuntime.EnsureServiceAccount(ctx, cluster); err != nil {
		return operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err))
	}
	if err := m.backupRuntime.EnsureRBAC(ctx, cluster); err != nil {
		return operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, fmt.Errorf("failed to ensure backup RBAC: %w", err))
	}
	return nil
}

func (m *Manager) resolvePreUpgradeBackupExecutorDigest(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (string, error) {
	executorImage := strings.TrimSpace(cluster.Spec.Backup.Image)
	if executorImage == "" || !security.IsOperatorImageVerificationEnabled(cluster) {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := security.VerifyOperatorImageForCluster(verifyCtx, logger, m.operatorImageVerifier, cluster, executorImage)
	if err != nil {
		if operatorImageVerificationFailurePolicy(cluster) == constants.ImageVerificationFailurePolicyBlock {
			return "", operatorerrors.WithReason(constants.ReasonPreUpgradeBackupImageVerificationFailed, fmt.Errorf("pre-upgrade backup executor image verification failed (policy=Block): %w", err))
		}

		logger.Error(err, "Pre-upgrade backup executor image verification failed but proceeding due to Warn policy", "image", executorImage)
		return "", nil
	}

	logger.Info("Pre-upgrade backup executor image verified successfully", "digest", digest)
	return digest, nil
}

func (m *Manager) buildPreUpgradeBackupJob(
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	verifiedExecutorDigest string,
) (*batchv1.Job, error) {
	job, err := m.backupRuntime.BuildPreUpgradeJob(cluster, portbackup.JobBuildOptions{
		JobName:                jobName,
		FilenamePrefix:         constants.BackupTypePreUpgrade,
		VerifiedExecutorDigest: verifiedExecutorDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build backup job: %w", err)
	}

	if m.scheme != nil {
		if err := controllerutil.SetControllerReference(cluster, job, m.scheme); err != nil {
			return nil, fmt.Errorf("failed to set owner reference on backup job: %w", err)
		}
	}

	return job, nil
}
