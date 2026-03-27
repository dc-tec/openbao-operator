package backup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

// Reconcile ensures backup configuration and status are aligned with the desired state for the given OpenBaoCluster.
// It checks if a backup is due, executes it if needed, and applies retention policies.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if cluster.Spec.Backup == nil {
		return recon.Result{}, nil
	}

	if err := validateBackupEgressConfiguration(cluster); err != nil {
		return recon.Result{}, err
	}

	logger = logger.WithValues("component", constants.ComponentBackup)
	metrics := NewMetrics(cluster.Namespace, cluster.Name)
	now := time.Now().UTC()

	if err := m.syncBackupMetrics(ctx, logger, cluster, metrics); err != nil {
		return recon.Result{}, err
	}
	if err := m.ensureBackupServiceAccount(ctx, logger, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err)
	}
	if err := m.ensureBackupRBAC(ctx, logger, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure backup RBAC: %w", err)
	}
	if err := m.ensureBackupStatus(ctx, cluster); err != nil {
		return recon.Result{}, err
	}

	schedule, err := ParseSchedule(cluster.Spec.Backup.Schedule)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to parse backup schedule: %w", err)
	}
	if cluster.Status.Backup.NextScheduledBackup == nil {
		next := metav1.NewTime(schedule.Next(now))
		cluster.Status.Backup.NextScheduledBackup = &next
	}

	manualTrigger, scheduledTime, err := m.handleManualTrigger(ctx, logger, cluster, now)
	if err != nil {
		return recon.Result{}, err
	}
	if !manualTrigger {
		scheduledTime = cluster.Status.Backup.NextScheduledBackup.Time
	}
	backupDue := manualTrigger || !now.Before(scheduledTime)

	if shouldSkip, err := m.handleRestoreInProgress(ctx, logger, cluster, backupDue); shouldSkip || err != nil {
		return recon.Result{}, err
	}

	if err := m.checkPreconditions(ctx, logger, cluster); err != nil {
		var preconditionErr *backupPreconditionError
		if errors.As(err, &preconditionErr) {
			logger.Info("Backup preconditions not met", "reason", preconditionErr.Error())
			if backupDue {
				m.emitPreconditionEvent(cluster, preconditionErr)
			}
			return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
		}
		return recon.Result{}, err
	}

	hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasActiveJob {
		logger.V(1).Info("Backup Job in progress; requeueing to observe completion")
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	shouldReturn, result, err := m.checkBackupDue(ctx, logger, cluster, now, scheduledTime, manualTrigger)
	if shouldReturn {
		return result, err
	}

	return m.executeAndProcessBackup(ctx, logger, cluster, schedule, metrics, now, scheduledTime, manualTrigger)
}

func (m *Manager) ensureBackupStatus(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.Backup != nil {
		return nil
	}

	cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to initialize backup status: %w", err)
	}
	return nil
}

func (m *Manager) handleRestoreInProgress(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, backupDue bool) (bool, error) {
	restoreInProgress, err := m.hasInProgressRestore(ctx, logger, cluster)
	if err != nil {
		return false, err
	}
	if !restoreInProgress {
		return false, nil
	}

	if backupOperationLock.IsHeldBy(cluster.Status.OperationLock) {
		hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
		if err != nil {
			return false, fmt.Errorf("failed to check for active backup job while restore is in progress: %w", err)
		}
		if !hasActiveJob {
			m.releaseBackupLock(ctx, logger, cluster, "while restore is in progress")
		}
	}
	if backupDue {
		m.emitNormalEvent(cluster, ReasonBackupSkipped, "Skipping backup because a restore is in progress for cluster %s", cluster.Name)
	}
	logger.Info("Restore in progress; skipping backup reconciliation")
	return true, nil
}

func (m *Manager) releaseBackupLock(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, contextNote string) {
	if err := opslifecycle.Release(ctx, m.client, cluster, backupOperationLock); err != nil && !opslifecycle.IsLockHeld(err) {
		logger.Error(err, "Failed to release backup operation lock", "context", contextNote)
		return
	} else if err == nil {
		logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationBackup),
			"holder":            backupOperationLockHolder,
		})
	}
}
