package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

// executeAndProcessBackup creates/checks the backup job and processes results.
func (m *Manager) executeAndProcessBackup(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	schedule cron.Schedule,
	metrics *Metrics,
	now time.Time,
	scheduledTime time.Time,
	manualTriggerToken string,
) (recon.Result, error) {
	manualTrigger := manualTriggerToken != ""
	nextScheduled := schedule.Next(scheduledTime)
	if !nextScheduled.After(now) {
		nextScheduled = schedule.Next(now)
	}

	jobName := backupJobName(cluster, scheduledTime)
	if manualTrigger {
		logger.Info("Manual backup triggered, ensuring backup Job", "job", jobName)
	} else {
		logger.Info("Backup is due, ensuring backup Job", "job", jobName)
	}
	metrics.SetInProgress(true)
	lockHeldByUs := backupOperationLock.IsHeldBy(cluster.Status.OperationLock)

	if blockedResult, err := m.acquireBackupLock(ctx, logger, cluster, jobName); err != nil {
		return recon.Result{}, err
	} else if blockedResult != nil {
		return *blockedResult, nil
	}
	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationBackup),
			"holder":            backupOperationLockHolder,
		})
		m.emitNormalEvent(cluster, ReasonBackupStarted, "Backup started for schedule %s", scheduledTime.UTC().Format(time.RFC3339))
	}

	jobInProgress, err := m.ensureBackupJob(ctx, logger, cluster, jobName, scheduledTime)
	if err != nil {
		m.releaseBackupLock(ctx, logger, cluster, "after job ensure failure")
		return recon.Result{}, fmt.Errorf("failed to ensure backup Job: %w", err)
	}

	if manualTrigger {
		m.clearTriggerAnnotation(ctx, logger, cluster, constants.AnnotationTriggerBackup)
	}

	if err := m.recordBackupAttempt(ctx, cluster, now, scheduledTime, nextScheduled, manualTriggerToken); err != nil {
		logger.Error(err, "Failed to record backup attempt")
	}

	if jobInProgress {
		if _, err := m.processBackupJobResult(ctx, logger, cluster, jobName); err != nil {
			return recon.Result{}, fmt.Errorf("failed to process backup Job result: %w", err)
		}
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	statusUpdated, err := m.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to process backup Job result: %w", err)
	}

	if cluster.Status.Backup != nil && cluster.Status.Backup.LastBackupTime != nil {
		if cluster.Spec.Backup.Retention != nil {
			if err := m.applyRetention(ctx, logger, cluster, metrics); err != nil {
				logger.Error(err, "Failed to apply retention policy")
			}
		}
		nextScheduledMeta := metav1.NewTime(nextScheduled)
		cluster.Status.Backup.NextScheduledBackup = &nextScheduledMeta
		if err := m.patchStatusSSA(ctx, cluster); err != nil {
			logger.Error(err, "Failed to patch backup status after retention")
		}
	}

	m.releaseBackupLock(ctx, logger, cluster, "after completion")
	if statusUpdated {
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}
	return recon.Result{RequeueAfter: time.Until(nextScheduled)}, nil
}

func (m *Manager) acquireBackupLock(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
) (*recon.Result, error) {
	if err := opslifecycle.AcquireWithReader(ctx, m.reader, m.client, cluster, backupOperationLock, opslifecycle.AcquireOptions{
		Message: fmt.Sprintf("backup job %s", jobName),
	}); err != nil {
		if opslifecycle.IsLockHeld(err) {
			fields := map[string]string{
				"cluster_namespace": cluster.Namespace,
				"cluster_name":      cluster.Name,
				"operation":         string(openbaov1alpha1.ClusterOperationBackup),
				"holder":            backupOperationLockHolder,
			}
			opslifecycle.AddHeldAuditFields(fields, err)
			logging.LogAuditEvent(logger, logging.EventOperationLockBlocked, fields)
			m.emitWarningEvent(cluster, ReasonOperationLockBlocked, "Backup blocked by operation lock: %v", err)
			logger.Info("Backup blocked by operation lock", "error", err.Error())
			result := recon.Result{RequeueAfter: opslifecycle.RequeueDelay(opslifecycle.RetryClassStandard)}
			return &result, nil
		}
		return nil, fmt.Errorf("failed to acquire backup operation lock: %w", err)
	}

	return nil, nil
}
