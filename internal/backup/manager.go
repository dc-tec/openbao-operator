// Package backup provides backup management for OpenBao clusters.
// It handles scheduled snapshots to object storage and retention policy enforcement.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/interfaces"
	"github.com/dc-tec/openbao-operator/internal/kube"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/storage"
)

// ErrNoBackupToken indicates that no suitable backup token is configured for
// the cluster. This occurs when neither JWT Auth role nor backup token Secret
// is provided, or the referenced Secret is missing.
var ErrNoBackupToken = errors.New("no backup token configured: either jwtAuthRole or tokenSecretRef must be set")

// Manager reconciles backup configuration and execution for an OpenBaoCluster.
type Manager struct {
	client                client.Client
	scheme                *runtime.Scheme
	clientConfig          openbao.ClientConfig
	operatorImageVerifier interfaces.ImageVerifier
	Platform              string
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme, clientConfig openbao.ClientConfig, operatorImageVerifier interfaces.ImageVerifier, platform string) *Manager {
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

	// If a restore is in progress for this cluster, do not start new backups.
	// This prevents scheduled backups from repeatedly acquiring the operation lock
	// and starving the restore controller.
	restoreInProgress, err := m.hasInProgressRestore(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if restoreInProgress {
		if cluster.Status.OperationLock != nil &&
			cluster.Status.OperationLock.Operation == openbaov1alpha1.ClusterOperationBackup &&
			cluster.Status.OperationLock.Holder == constants.ControllerNameOpenBaoCluster+"/backup" {
			hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
			if err != nil {
				return recon.Result{}, fmt.Errorf("failed to check for active backup job while restore is in progress: %w", err)
			}
			if !hasActiveJob {
				if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
					logger.Error(err, "Failed to release backup operation lock while restore is in progress")
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

// handleManualTrigger checks for and handles manual backup trigger annotation.
// Returns (manualTrigger, scheduledTime, error).
func (m *Manager) handleManualTrigger(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	now time.Time,
) (bool, time.Time, error) {
	triggerAnnotation := constants.AnnotationTriggerBackup
	val, ok := cluster.Annotations[triggerAnnotation]
	if !ok || val == "" {
		return false, time.Time{}, nil
	}

	logger.Info("Manual backup trigger detected", "annotation", val)

	// Check if there's already a backup job in progress
	hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasActiveJob {
		logger.Info("Manual backup triggered but job already in progress, skipping duplicate")
		m.clearTriggerAnnotation(ctx, logger, cluster, triggerAnnotation)
		return false, time.Time{}, nil
	}

	return true, now, nil
}

// clearTriggerAnnotation removes the manual trigger annotation from the cluster.
func (m *Manager) clearTriggerAnnotation(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, annotation string) {
	// We use MergeFrom for annotation deletion to avoid claiming ownership of all annotations
	original := cluster.DeepCopy()
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	delete(cluster.Annotations, annotation)
	if err := m.client.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		logger.Error(err, "Failed to clear manual backup trigger annotation")
	} else {
		logger.Info("Cleared manual backup trigger annotation")
	}
}

// patchStatusSSA updates the backup status using Server-Side Apply.
func (m *Manager) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup: cluster.Status.Backup,
		},
	}

	return m.client.Status().Patch(ctx, applyCluster, client.Apply,
		client.FieldOwner("openbao-backup-controller"),
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
		if cluster.Status.OperationLock != nil &&
			cluster.Status.OperationLock.Operation == openbaov1alpha1.ClusterOperationBackup &&
			cluster.Status.OperationLock.Holder == constants.ControllerNameOpenBaoCluster+"/backup" {
			if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
				logger.Error(err, "Failed to release backup operation lock after completed Job processing")
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

	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    constants.ControllerNameOpenBaoCluster + "/backup",
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Message:   fmt.Sprintf("backup job %s", jobName),
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			logger.Info("Backup blocked by operation lock", "error", err.Error())
			return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to acquire backup operation lock: %w", err)
	}

	// Create or check backup Job
	jobInProgress, err := m.ensureBackupJob(ctx, logger, cluster, jobName, scheduledTime)
	if err != nil {
		if releaseErr := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); releaseErr != nil && !errors.Is(releaseErr, operationlock.ErrLockHeld) {
			logger.Error(releaseErr, "Failed to release backup operation lock after job ensure failure")
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

	if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
		logger.Error(err, "Failed to release backup operation lock after completion")
	}

	if statusUpdated {
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}
	return recon.Result{RequeueAfter: time.Until(nextScheduled)}, nil
}

func (m *Manager) recordBackupAttempt(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, now time.Time, scheduledTime time.Time, nextScheduled time.Time) error {
	if cluster.Status.Backup == nil {
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	}

	nowMeta := metav1.NewTime(now)
	cluster.Status.Backup.LastAttemptTime = &nowMeta

	scheduledMeta := metav1.NewTime(scheduledTime)
	cluster.Status.Backup.LastAttemptScheduledTime = &scheduledMeta

	nextScheduledMeta := metav1.NewTime(nextScheduled)
	cluster.Status.Backup.NextScheduledBackup = &nextScheduledMeta

	return m.patchStatusSSA(ctx, cluster)
}

// BackupResult contains the result of a successful backup.
type BackupResult struct {
	// Key is the object storage key where the backup was stored.
	Key string
	// Size is the size of the backup in bytes.
	Size int64
}

func (m *Manager) hasInProgressRestore(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	restoreList := &openbaov1alpha1.OpenBaoRestoreList{}
	if err := m.client.List(ctx, restoreList, client.InNamespace(cluster.Namespace)); err != nil {
		// In some zero-trust deployments, the OpenBaoCluster controller may not have permissions
		// to list restore resources. Fallback to "no restore detected" in that case.
		if apierrors.IsForbidden(err) {
			logger.V(1).Info("Insufficient permissions to list OpenBaoRestore resources; cannot detect restore-in-progress", "error", err.Error())
			return false, nil
		}
		return false, fmt.Errorf("failed to list OpenBaoRestore resources: %w", err)
	}

	for i := range restoreList.Items {
		restore := &restoreList.Items[i]
		if restore.DeletionTimestamp != nil {
			continue
		}
		if restore.Spec.Cluster != cluster.Name {
			continue
		}
		if restore.Status.Phase == openbaov1alpha1.RestorePhaseCompleted ||
			restore.Status.Phase == openbaov1alpha1.RestorePhaseFailed {
			continue
		}
		return true, nil
	}

	return false, nil
}

// checkPreconditions verifies that backup can proceed.
func (m *Manager) checkPreconditions(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Check cluster is initialized
	if !cluster.Status.Initialized {
		return fmt.Errorf("cluster is not initialized")
	}

	// Check cluster phase - don't backup during initialization
	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseInitializing {
		return fmt.Errorf("cluster is initializing")
	}

	// Check if an upgrade is about to start or in progress
	// This prevents regular backups from starting when an upgrade is detected or in progress
	// We use the same logic as the upgrade manager to detect pending upgrades
	// This catches upgrades before Status.Upgrade is set and before pre-upgrade jobs are visible
	if cluster.Status.Initialized {
		// Only check for pending upgrades if cluster is initialized
		// (upgrade manager also skips if not initialized)
		if cluster.Status.CurrentVersion != "" {
			// CurrentVersion is set - check if it differs from spec
			if cluster.Spec.Version != cluster.Status.CurrentVersion {
				// Upgrade is about to start - check if pre-upgrade snapshot is enabled
				if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.PreUpgradeSnapshot {
					// Pre-upgrade snapshot is enabled - skip regular backups
					// The upgrade manager will handle the pre-upgrade backup
					return fmt.Errorf("upgrade pending with pre-upgrade snapshot enabled")
				}
				// Upgrade is about to start but no pre-upgrade snapshot - still skip regular backups
				return fmt.Errorf("upgrade pending")
			}
		}
		// If CurrentVersion is empty but cluster is initialized, this is the first reconcile after init
		// The upgrade manager will set CurrentVersion, so no upgrade is pending yet
	}

	// Check if upgrade is in progress - skip scheduled backups during upgrades
	// Exception: Pre-upgrade backups are triggered by the upgrade manager, not here
	if cluster.Status.Upgrade != nil {
		return fmt.Errorf("upgrade in progress")
	}

	// Check if a pre-upgrade backup job exists or is in progress
	// This is a fallback check in case the version check above didn't catch it
	// (e.g., if Status.CurrentVersion is empty or there's a timing issue)
	hasPreUpgradeJob, err := m.hasPreUpgradeBackupJob(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to check for pre-upgrade backup job: %w", err)
	}
	if hasPreUpgradeJob {
		return fmt.Errorf("pre-upgrade backup in progress")
	}

	// Check if another backup is in progress
	hasBackupJob, err := m.hasActiveBackupJob(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasBackupJob {
		return fmt.Errorf("backup already in progress")
	}

	// Check we have a token for backup.
	// All clusters (both standard and self-init) must use either JWT Auth
	// or a backup token Secret. Root tokens are not used for security reasons.
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return ErrNoBackupToken
	}

	// Check if JWT Auth is configured (preferred method)
	hasJWTAuth := strings.TrimSpace(backupCfg.JWTAuthRole) != ""

	// Check if static token is configured (fallback method)
	hasTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""

	// At least one authentication method must be configured
	if !hasJWTAuth && !hasTokenSecret {
		return ErrNoBackupToken
	}

	// If using token secret, verify it exists
	// SECURITY: Always use cluster.Namespace for secret lookups.
	// Do NOT trust user-provided Namespace in SecretRef to prevent Confused Deputy attacks.
	if hasTokenSecret {
		secretNamespace := cluster.Namespace

		secretName := types.NamespacedName{
			Namespace: secretNamespace,
			Name:      backupCfg.TokenSecretRef.Name,
		}

		secret := &corev1.Secret{}
		if err := m.client.Get(ctx, secretName, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("backup token Secret %s/%s not found: %w", secretNamespace, backupCfg.TokenSecretRef.Name, ErrNoBackupToken)
			}
			return fmt.Errorf("failed to get backup token Secret %s/%s: %w", secretNamespace, backupCfg.TokenSecretRef.Name, err)
		}
	}

	return nil
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

	// Load storage credentials
	creds, err := kube.LoadStorageCredentials(ctx, m.client, cluster.Spec.Backup.Target.CredentialsSecretRef, cluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to load storage credentials for retention: %w", err)
	}

	// Create storage client
	storageClient, err := storage.OpenS3Bucket(ctx, storage.S3ClientConfig{
		Endpoint:        cluster.Spec.Backup.Target.Endpoint,
		Bucket:          cluster.Spec.Backup.Target.Bucket,
		Region:          creds.Region,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		CACert:          creds.CACert,
		UsePathStyle:    cluster.Spec.Backup.Target.UsePathStyle,
	})
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

// ensureBackupServiceAccount creates or updates the ServiceAccount for backup Jobs using Server-Side Apply.
// This ServiceAccount is used for JWT Auth authentication to OpenBao.
// ensureBackupServiceAccount creates or updates the ServiceAccount for backup Jobs using Server-Side Apply.
// This ServiceAccount is used for JWT Auth authentication to OpenBao.
func (m *Manager) ensureBackupServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster)
}

// ensureBackupRBAC creates a Role and RoleBinding that grants the backup service account
// permission to list pods in its namespace. This is required for finding the active OpenBao pod.
func (m *Manager) ensureBackupRBAC(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureBackupRBAC(ctx, m.client, m.scheme, cluster)
}

// backupLabels returns the labels for backup resources
func backupLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	}
}

// backupServiceAccountName returns the name for the backup ServiceAccount.
func backupServiceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixBackupServiceAccount
}

// hasPreUpgradeBackupJob checks if there's a pre-upgrade backup job running or pending for this cluster.
// This is used to prevent regular scheduled backups from starting when an upgrade is initiating.
func (m *Manager) hasPreUpgradeBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:       cluster.Name,
		constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:    cluster.Name,
		constants.LabelOpenBaoComponent:  ComponentBackup,
		constants.LabelOpenBaoBackupType: "pre-upgrade",
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list pre-upgrade backup jobs: %w", err)
	}

	// Check if there's a running or pending job (not yet succeeded or failed)
	for i := range jobList.Items {
		job := &jobList.Items[i]
		// If job hasn't succeeded or failed, it's still running or pending
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			return true, nil
		}
	}

	return false, nil
}

// checkForCompletedJobs checks for any completed backup jobs and processes them.
// Returns (statusUpdated, error) where statusUpdated indicates if any job was processed and status was updated.
// This is used to ensure completed jobs are processed even when backup is not due yet.
// Only the most recent completed job is processed to avoid incrementing ConsecutiveFailures multiple times
// when there are several old failed jobs.
func (m *Manager) checkForCompletedJobs(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list backup jobs: %w", err)
	}

	// Find the most recent completed job (by creation timestamp).
	// We only process the most recent one to avoid processing stale failures repeatedly.
	var mostRecentCompleted *batchv1.Job
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			continue // Skip jobs that are still running
		}
		if mostRecentCompleted == nil || job.CreationTimestamp.After(mostRecentCompleted.CreationTimestamp.Time) {
			mostRecentCompleted = job
		}
	}

	if mostRecentCompleted == nil {
		return false, nil // No completed jobs to process
	}

	logger.Info("Processing completed backup job", "job", mostRecentCompleted.Name,
		"succeeded", mostRecentCompleted.Status.Succeeded, "failed", mostRecentCompleted.Status.Failed)

	statusUpdated, err := m.processBackupJobResult(ctx, logger, cluster, mostRecentCompleted.Name)
	if err != nil {
		return false, err
	}
	if statusUpdated {
		logger.Info("Completed backup job processed, status updated", "job", mostRecentCompleted.Name)
	} else {
		logger.V(1).Info("Completed backup job already processed", "job", mostRecentCompleted.Name)
	}

	return statusUpdated, nil
}

// hasActiveBackupJob checks if there's any backup job (scheduled or manual) running or pending for this cluster.
// This is used to prevent duplicate jobs from being created when manual triggers are processed multiple times.
func (m *Manager) hasActiveBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list backup jobs: %w", err)
	}

	// Check if there's a running or pending job (not yet succeeded or failed)
	for i := range jobList.Items {
		job := &jobList.Items[i]
		// If job hasn't succeeded or failed, it's still running or pending
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			return true, nil
		}
	}

	return false, nil
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
