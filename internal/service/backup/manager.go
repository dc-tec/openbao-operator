// Package backup provides backup management for OpenBao clusters.
// It handles scheduled snapshots to object storage and retention policy enforcement.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/logging"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/opslifecycle"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

// ErrNoBackupToken indicates that no suitable backup token is configured for
// the cluster. This occurs when neither JWT Auth role nor backup token Secret
// is provided, or the referenced Secret is missing.
var ErrNoBackupToken = errors.New("no backup token configured: either jwtAuthRole or tokenSecretRef must be set")

const backupOperationLockHolder = constants.ControllerNameOpenBaoCluster + "/backup"

var backupOperationLock = opslifecycle.OperationLock{
	Holder:    backupOperationLockHolder,
	Operation: openbaov1alpha1.ClusterOperationBackup,
}

// Manager reconciles backup configuration and execution for an OpenBaoCluster.
type Manager struct {
	client                client.Client
	scheme                *runtime.Scheme
	clientConfig          openbao.ClientConfig
	operatorImageVerifier imageverify.Verifier
	Platform              string
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme, clientConfig openbao.ClientConfig, operatorImageVerifier imageverify.Verifier, platform string) *Manager {
	return &Manager{
		client:                c,
		scheme:                scheme,
		clientConfig:          clientConfig,
		operatorImageVerifier: operatorImageVerifier,
		Platform:              platform,
	}
}

// Reconcile ensures backup configuration and status are aligned with the desired state for the given OpenBaoCluster.
// It checks if a backup is due, executes it if needed, and applies retention policies.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	// Skip if backup is not configured
	if cluster.Spec.Backup == nil {
		return recon.Result{}, nil
	}

	if err := validateBackupEgressConfiguration(cluster); err != nil {
		return recon.Result{}, err
	}

	logger = logger.WithValues("component", constants.ComponentBackup)
	metrics := NewMetrics(cluster.Namespace, cluster.Name)
	now := time.Now().UTC()

	// Keep backup metrics aligned with observed status and Jobs so dashboards remain stable
	// even when backups are infrequent.
	if err := m.syncBackupMetrics(ctx, logger, cluster, metrics); err != nil {
		return recon.Result{}, err
	}

	// If a restore is in progress for this cluster, do not start new backups.
	// This prevents scheduled backups from repeatedly acquiring the operation lock
	// and starving the restore controller.
	restoreInProgress, err := m.hasInProgressRestore(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if restoreInProgress {
		if backupOperationLock.IsHeldBy(cluster.Status.OperationLock) {
			hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
			if err != nil {
				return recon.Result{}, fmt.Errorf("failed to check for active backup job while restore is in progress: %w", err)
			}
			if !hasActiveJob {
				if err := opslifecycle.Release(ctx, m.client, cluster, backupOperationLock); err != nil && !opslifecycle.IsLockHeld(err) {
					logger.Error(err, "Failed to release backup operation lock while restore is in progress")
				} else if err == nil {
					logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
						"cluster_namespace": cluster.Namespace,
						"cluster_name":      cluster.Name,
						"operation":         string(openbaov1alpha1.ClusterOperationBackup),
						"holder":            backupOperationLockHolder,
					})
				}
			}
		}
		logger.Info("Restore in progress; skipping backup reconciliation")
		return recon.Result{}, nil
	}

	// Ensure backup ServiceAccount exists (for JWT Auth)
	if err := m.ensureBackupServiceAccount(ctx, logger, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err)
	}

	// Ensure backup RBAC exists (for pod listing/leader discovery)
	if err := m.ensureBackupRBAC(ctx, logger, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure backup RBAC: %w", err)
	}

	// Initialize backup status if needed
	if cluster.Status.Backup == nil {
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
		if err := m.patchStatusSSA(ctx, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to initialize backup status: %w", err)
		}
	}

	// Parse schedule and set NextScheduledBackup
	schedule, err := ParseSchedule(cluster.Spec.Backup.Schedule)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to parse backup schedule: %w", err)
	}
	if cluster.Status.Backup.NextScheduledBackup == nil {
		next := schedule.Next(now)
		nextMeta := metav1.NewTime(next)
		cluster.Status.Backup.NextScheduledBackup = &nextMeta
	}

	// Check for manual backup trigger
	manualTrigger, scheduledTime, err := m.handleManualTrigger(ctx, logger, cluster, now)
	if err != nil {
		return recon.Result{}, err
	}
	if !manualTrigger {
		scheduledTime = cluster.Status.Backup.NextScheduledBackup.Time
	}

	// Pre-flight checks
	if err := m.checkPreconditions(ctx, logger, cluster); err != nil {
		logger.Info("Backup preconditions not met", "reason", err.Error())
		return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
	}

	// If a Job is already running/pending, poll it to observe completion and release locks promptly.
	hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasActiveJob {
		logger.V(1).Info("Backup Job in progress; requeueing to observe completion")
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	// Check if backup is due
	shouldReturn, result, err := m.checkBackupDue(ctx, logger, cluster, schedule, now, scheduledTime, manualTrigger)
	if shouldReturn {
		return result, err
	}

	// Execute backup and process results
	return m.executeAndProcessBackup(ctx, logger, cluster, schedule, metrics, now, scheduledTime, manualTrigger)
}

func validateBackupEgressConfiguration(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Backup == nil {
		return nil
	}

	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		return nil
	}

	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		return nil
	}

	return operatorerrors.WithReason(
		constants.ReasonNetworkEgressRulesRequired,
		operatorerrors.WrapPermanentConfig(fmt.Errorf(
			"hardened profile with backups enabled requires explicit spec.network.egressRules so backup Jobs can reach the object storage endpoint",
		)),
	)
}

// checkBackupDue determines if a backup should be executed now.
// Returns (shouldReturn, result, error) where shouldReturn indicates early return.
//
//nolint:unparam // schedule parameter kept for API consistency with executeAndProcessBackup
func (m *Manager) checkBackupDue(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	_ cron.Schedule, // schedule parameter unused but kept for API consistency
	now time.Time,
	scheduledTime time.Time,
	manualTrigger bool,
) (bool, recon.Result, error) {
	if manualTrigger || !now.Before(scheduledTime) {
		return false, recon.Result{}, nil // Backup is due
	}

	timeUntilDue := scheduledTime.Sub(now)
	logger.V(1).Info("Backup not due yet", "scheduledTime", scheduledTime, "now", now, "timeUntilDue", timeUntilDue)

	// Check for completed jobs
	statusUpdated, err := m.checkForCompletedJobs(ctx, logger, cluster)
	if err != nil {
		return true, recon.Result{}, fmt.Errorf("failed to check for completed backup jobs: %w", err)
	}
	if statusUpdated {
		logger.Info("Found completed backup job, requesting requeue to persist status")
		if backupOperationLock.IsHeldBy(cluster.Status.OperationLock) {
			if err := opslifecycle.Release(ctx, m.client, cluster, backupOperationLock); err != nil && !opslifecycle.IsLockHeld(err) {
				logger.Error(err, "Failed to release backup operation lock after completed Job processing")
			} else if err == nil {
				logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
					"cluster_namespace": cluster.Namespace,
					"cluster_name":      cluster.Name,
					"operation":         string(openbaov1alpha1.ClusterOperationBackup),
					"holder":            backupOperationLockHolder,
				})
			}
		}
		return true, recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	return true, recon.Result{RequeueAfter: timeUntilDue}, nil
}

// executeAndProcessBackup creates/checks the backup job and processes results.
func (m *Manager) executeAndProcessBackup(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	schedule cron.Schedule,
	metrics *Metrics,
	now time.Time,
	scheduledTime time.Time,
	manualTrigger bool,
) (recon.Result, error) {
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

	if err := opslifecycle.Acquire(ctx, m.client, cluster, backupOperationLock, opslifecycle.AcquireOptions{
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
			logger.Info("Backup blocked by operation lock", "error", err.Error())
			return recon.Result{RequeueAfter: opslifecycle.RequeueDelay(opslifecycle.RetryClassStandard)}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to acquire backup operation lock: %w", err)
	}
	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationBackup),
			"holder":            backupOperationLockHolder,
		})
	}

	// Create or check backup Job
	jobInProgress, err := m.ensureBackupJob(ctx, logger, cluster, jobName, scheduledTime)
	if err != nil {
		if releaseErr := opslifecycle.Release(ctx, m.client, cluster, backupOperationLock); releaseErr != nil && !opslifecycle.IsLockHeld(releaseErr) {
			logger.Error(releaseErr, "Failed to release backup operation lock after job ensure failure")
		} else if releaseErr == nil {
			logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
				"cluster_namespace": cluster.Namespace,
				"cluster_name":      cluster.Name,
				"operation":         string(openbaov1alpha1.ClusterOperationBackup),
				"holder":            backupOperationLockHolder,
			})
		}
		return recon.Result{}, fmt.Errorf("failed to ensure backup Job: %w", err)
	}

	// Clear manual trigger annotation after job creation
	if manualTrigger {
		m.clearTriggerAnnotation(ctx, logger, cluster, constants.AnnotationTriggerBackup)
	}

	if err := m.recordBackupAttempt(ctx, cluster, now, scheduledTime, nextScheduled); err != nil {
		logger.Error(err, "Failed to record backup attempt")
		// Continue even if recording attempt fails, as the job is created
	}

	if jobInProgress {
		_, err := m.processBackupJobResult(ctx, logger, cluster, jobName)
		if err != nil {
			return recon.Result{}, fmt.Errorf("failed to process backup Job result: %w", err)
		}
		// The OpenBaoCluster controller does not watch Job resources (zero-trust model),
		// so we must request requeues while a backup Job is running to observe completion
		// and release the operation lock promptly.
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	// Process completed job
	statusUpdated, err := m.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		return recon.Result{}, fmt.Errorf("failed to process backup Job result: %w", err)
	}

	// Apply retention if backup completed
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

	if err := opslifecycle.Release(ctx, m.client, cluster, backupOperationLock); err != nil && !opslifecycle.IsLockHeld(err) {
		logger.Error(err, "Failed to release backup operation lock after completion")
	} else if err == nil {
		logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationBackup),
			"holder":            backupOperationLockHolder,
		})
	}

	if statusUpdated {
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}
	return recon.Result{RequeueAfter: time.Until(nextScheduled)}, nil
}

// BackupResult contains the result of a successful backup.
type BackupResult struct {
	// Key is the object storage key where the backup was stored.
	Key string
	// Size is the size of the backup in bytes.
	Size int64
}

// applyRetention applies the retention policy after a successful backup.
func (m *Manager) applyRetention(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *Metrics) error {
	retention := cluster.Spec.Backup.Retention
	if retention == nil {
		return nil
	}

	if cluster.Spec.Backup.Target.RoleARN != "" {
		// When backups use workload identity (Web Identity / OIDC federation), the
		// controller process is intentionally not granted object storage access.
		// Prefer enforcing retention via storage-native lifecycle policies.
		logger.Info("Skipping retention for workload identity backup target",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name)
		return nil
	}

	if cluster.Spec.Backup.Target.CredentialsSecretRef == nil {
		logger.Info("Skipping retention because no storage credentials Secret is configured",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name)
		return nil
	}

	// Parse MaxAge duration
	maxAge, err := ParseRetentionMaxAge(retention.MaxAge)
	if err != nil {
		return fmt.Errorf("failed to parse retention maxAge: %w", err)
	}

	policy := RetentionPolicy{
		MaxCount: retention.MaxCount,
		MaxAge:   maxAge,
	}

	// Create storage client
	storageClient, err := m.openBackupStorageClient(ctx, cluster, false)
	if err != nil {
		return fmt.Errorf("failed to create storage client for retention: %w", err)
	}
	defer func() {
		_ = storageClient.Close()
	}()

	// Get backup list prefix
	prefix := GetBackupListPrefix(
		cluster.Spec.Backup.Target.PathPrefix,
		cluster.Namespace,
		cluster.Name,
	)

	result, err := ApplyRetention(ctx, logger, storageClient, prefix, policy)
	if err != nil {
		return err
	}

	// Record metrics
	totalDeleted := result.DeletedByCount + result.DeletedByAge
	if totalDeleted > 0 {
		metrics.IncrementRetentionDeleted(totalDeleted)
	}

	return nil
}

// countingReader wraps an io.Reader to count bytes read.
type countingReader struct {
	reader    io.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += int64(n)
	return n, err
}
