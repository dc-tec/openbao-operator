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
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
	"github.com/dc-tec/openbao-operator/internal/storage"
)

// ErrNoBackupToken indicates that no suitable backup token is configured for
// the cluster. This occurs when neither JWT Auth role nor backup token Secret
// is provided, or the referenced Secret is missing.
var ErrNoBackupToken = errors.New("no backup token configured: either jwtAuthRole or tokenSecretRef must be set")

// Manager reconciles backup configuration and execution for an OpenBaoCluster.
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: scheme,
	}
}

// Reconcile ensures backup configuration and status are aligned with the desired state for the given OpenBaoCluster.
// It checks if a backup is due, executes it if needed, and applies retention policies.
// Returns (shouldRequeue, error) where shouldRequeue indicates if reconciliation should be requeued immediately.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	// Skip if backup is not configured
	if cluster.Spec.Backup == nil {
		return false, nil
	}

	logger = logger.WithValues("component", constants.ComponentBackup)
	metrics := NewMetrics(cluster.Namespace, cluster.Name)
	now := time.Now().UTC()

	// If a restore is in progress for this cluster, do not start new backups.
	// This prevents scheduled backups from repeatedly acquiring the operation lock
	// and starving the restore controller.
	restoreInProgress, err := m.hasInProgressRestore(ctx, logger, cluster)
	if err != nil {
		return false, err
	}
	if restoreInProgress {
		if cluster.Status.OperationLock != nil &&
			cluster.Status.OperationLock.Operation == openbaov1alpha1.ClusterOperationBackup &&
			cluster.Status.OperationLock.Holder == constants.ControllerNameOpenBaoCluster+"/backup" {
			hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
			if err != nil {
				return false, fmt.Errorf("failed to check for active backup job while restore is in progress: %w", err)
			}
			if !hasActiveJob {
				if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
					logger.Error(err, "Failed to release backup operation lock while restore is in progress")
				}
			}
		}
		logger.Info("Restore in progress; skipping backup reconciliation")
		return false, nil
	}

	// Ensure backup ServiceAccount exists (for JWT Auth)
	if err := m.ensureBackupServiceAccount(ctx, logger, cluster); err != nil {
		return false, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err)
	}

	// Ensure backup RBAC exists (for pod listing/leader discovery)
	if err := m.ensureBackupRBAC(ctx, logger, cluster); err != nil {
		return false, fmt.Errorf("failed to ensure backup RBAC: %w", err)
	}

	// Initialize backup status if needed
	if cluster.Status.Backup == nil {
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	}

	// Parse schedule and set NextScheduledBackup
	schedule, err := ParseSchedule(cluster.Spec.Backup.Schedule)
	if err != nil {
		return false, fmt.Errorf("failed to parse backup schedule: %w", err)
	}
	if cluster.Status.Backup.NextScheduledBackup == nil {
		next := schedule.Next(now)
		nextMeta := metav1.NewTime(next)
		cluster.Status.Backup.NextScheduledBackup = &nextMeta
	}

	// Check for manual backup trigger
	manualTrigger, scheduledTime, shouldReturn, result, err := m.handleManualTrigger(ctx, logger, cluster, now)
	if shouldReturn {
		return result, err
	}
	if !manualTrigger {
		scheduledTime = cluster.Status.Backup.NextScheduledBackup.Time
	}

	// Pre-flight checks
	if err := m.checkPreconditions(ctx, logger, cluster); err != nil {
		logger.Info("Backup preconditions not met", "reason", err.Error())
		return false, nil
	}

	// Check if backup is due
	shouldReturn, result, err = m.checkBackupDue(ctx, logger, cluster, schedule, now, scheduledTime, manualTrigger)
	if shouldReturn {
		return result, err
	}

	// Execute backup and process results
	return m.executeAndProcessBackup(ctx, logger, cluster, schedule, metrics, now, scheduledTime, manualTrigger)
}

// handleManualTrigger checks for and handles manual backup trigger annotation.
// Returns (manualTrigger, scheduledTime, shouldReturn, result, error).
//
//nolint:unparam // result (4th return) is always false but matches interface pattern
func (m *Manager) handleManualTrigger(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	now time.Time,
) (bool, time.Time, bool, bool, error) {
	triggerAnnotation := constants.AnnotationTriggerBackup
	val, ok := cluster.Annotations[triggerAnnotation]
	if !ok || val == "" {
		return false, time.Time{}, false, false, nil
	}

	logger.Info("Manual backup trigger detected", "annotation", val)

	// Check if there's already a backup job in progress
	hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
	if err != nil {
		return false, time.Time{}, true, false, fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasActiveJob {
		logger.Info("Manual backup triggered but job already in progress, skipping duplicate")
		m.clearTriggerAnnotation(ctx, logger, cluster, triggerAnnotation)
		return false, time.Time{}, true, false, nil
	}

	return true, now, false, false, nil
}

// clearTriggerAnnotation removes the manual trigger annotation from the cluster.
func (m *Manager) clearTriggerAnnotation(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, annotation string) {
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
) (bool, bool, error) {
	if manualTrigger || !now.Before(scheduledTime) {
		return false, false, nil // Backup is due
	}

	timeUntilDue := scheduledTime.Sub(now)
	logger.V(1).Info("Backup not due yet", "scheduledTime", scheduledTime, "now", now, "timeUntilDue", timeUntilDue)

	// Check for completed jobs
	statusUpdated, err := m.checkForCompletedJobs(ctx, logger, cluster)
	if err != nil {
		return true, false, fmt.Errorf("failed to check for completed backup jobs: %w", err)
	}
	if statusUpdated {
		logger.Info("Found completed backup job, requesting requeue to persist status")
		return true, true, nil
	}

	// Determine if requeue is needed based on schedule
	scheduleInterval, err := GetScheduleInterval(cluster.Spec.Backup.Schedule)
	if err == nil && timeUntilDue <= scheduleInterval {
		logger.V(1).Info("Backup due soon, requesting requeue", "timeUntilDue", timeUntilDue)
		return true, true, nil
	}
	if err != nil && timeUntilDue <= 5*time.Minute {
		logger.V(1).Info("Backup due soon, requesting requeue (fallback)", "timeUntilDue", timeUntilDue)
		return true, true, nil
	}

	return true, false, nil
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
) (bool, error) {
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
			return false, nil
		}
		return false, fmt.Errorf("failed to acquire backup operation lock: %w", err)
	}

	// Create or check backup Job
	jobInProgress, err := m.ensureBackupJob(ctx, logger, cluster, jobName, scheduledTime)
	if err != nil {
		if releaseErr := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); releaseErr != nil && !errors.Is(releaseErr, operationlock.ErrLockHeld) {
			logger.Error(releaseErr, "Failed to release backup operation lock after job ensure failure")
		}
		return false, fmt.Errorf("failed to ensure backup Job: %w", err)
	}

	// Clear manual trigger annotation after job creation
	if manualTrigger {
		m.clearTriggerAnnotation(ctx, logger, cluster, constants.AnnotationTriggerBackup)
	}

	m.recordBackupAttempt(cluster, now, scheduledTime, nextScheduled)

	if jobInProgress {
		_, err := m.processBackupJobResult(ctx, logger, cluster, jobName)
		if err != nil {
			return false, fmt.Errorf("failed to process backup Job result: %w", err)
		}
		// The OpenBaoCluster controller does not watch Job resources (zero-trust model),
		// so we must request requeues while a backup Job is running to observe completion
		// and release the operation lock promptly.
		return true, nil
	}

	// Process completed job
	statusUpdated, err := m.processBackupJobResult(ctx, logger, cluster, jobName)
	if err != nil {
		return false, fmt.Errorf("failed to process backup Job result: %w", err)
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
	}

	if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/backup", openbaov1alpha1.ClusterOperationBackup); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
		logger.Error(err, "Failed to release backup operation lock after completion")
	}

	return statusUpdated, nil
}

func (m *Manager) recordBackupAttempt(cluster *openbaov1alpha1.OpenBaoCluster, now time.Time, scheduledTime time.Time, nextScheduled time.Time) {
	if cluster.Status.Backup == nil {
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	}

	nowMeta := metav1.NewTime(now)
	cluster.Status.Backup.LastAttemptTime = &nowMeta

	scheduledMeta := metav1.NewTime(scheduledTime)
	cluster.Status.Backup.LastAttemptScheduledTime = &scheduledMeta

	nextScheduledMeta := metav1.NewTime(nextScheduled)
	cluster.Status.Backup.NextScheduledBackup = &nextScheduledMeta
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
	creds, err := storage.LoadCredentials(ctx, m.client, cluster.Spec.Backup.Target.CredentialsSecretRef, cluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to load storage credentials for retention: %w", err)
	}

	// Create storage client
	usePathStyle := cluster.Spec.Backup.Target.UsePathStyle
	partSize := cluster.Spec.Backup.Target.PartSize
	concurrency := cluster.Spec.Backup.Target.Concurrency
	storageClient, err := storage.NewS3ClientFromCredentials(
		ctx,
		cluster.Spec.Backup.Target.Endpoint,
		cluster.Spec.Backup.Target.Bucket,
		creds,
		usePathStyle,
		partSize,
		concurrency,
	)
	if err != nil {
		return fmt.Errorf("failed to create storage client for retention: %w", err)
	}

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

// applyResource uses Server-Side Apply to create or update a Kubernetes resource.
// This eliminates the need for Get-then-Create-or-Update logic and manual diffing.
//
// The resource must have TypeMeta, ObjectMeta (with Name and Namespace), and the desired Spec set.
// Owner references are set automatically if the resource supports them.
//
// fieldOwner identifies the operator as the manager of this resource (used for conflict resolution).
func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster, fieldOwner string) error {
	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(cluster, obj, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Use Server-Side Apply with ForceOwnership to ensure the operator manages this resource
	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(fieldOwner),
	}

	if err := m.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

// ensureBackupServiceAccount creates or updates the ServiceAccount for backup Jobs using Server-Side Apply.
// This ServiceAccount is used for JWT Auth authentication to OpenBao.
// ensureBackupServiceAccount creates or updates the ServiceAccount for backup Jobs using Server-Side Apply.
// This ServiceAccount is used for JWT Auth authentication to OpenBao.
func (m *Manager) ensureBackupServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := backupServiceAccountName(cluster)

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cluster.Namespace,
			Labels:    backupLabels(cluster),
		},
	}

	if err := m.applyResource(ctx, sa, cluster, "openbao-operator"); err != nil {
		return fmt.Errorf("failed to ensure backup ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// ensureBackupRBAC creates a Role and RoleBinding that grants the backup service account
// permission to list pods in its namespace. This is required for finding the active OpenBao pod.
func (m *Manager) ensureBackupRBAC(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := backupServiceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"
	resourceLabels := backupLabels(cluster)

	// Ensure Role exists using SSA
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: cluster.Namespace,
			Labels:    resourceLabels,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	if err := m.applyResource(ctx, role, cluster, "openbao-operator"); err != nil {
		return fmt.Errorf("failed to ensure Role %s/%s: %w", cluster.Namespace, roleName, err)
	}

	// Ensure RoleBinding exists using SSA
	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: cluster.Namespace,
			Labels:    resourceLabels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: cluster.Namespace,
			},
		},
	}

	if err := m.applyResource(ctx, roleBinding, cluster, "openbao-operator"); err != nil {
		return fmt.Errorf("failed to ensure RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
	}

	return nil
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
		if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			return true, nil
		}
	}

	return false, nil
}

// checkForCompletedJobs checks for any completed backup jobs and processes them.
// Returns (statusUpdated, error) where statusUpdated indicates if any job was processed and status was updated.
// This is used to ensure completed jobs are processed even when backup is not due yet.
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

	// Check for completed jobs (succeeded or failed) and process them
	// Process all completed jobs, but return true if any status was updated
	anyStatusUpdated := false
	for i := range jobList.Items {
		job := &jobList.Items[i]
		// Only process jobs that have completed (succeeded or failed)
		if job.Status.Succeeded > 0 || job.Status.Failed > 0 {
			logger.Info("Processing completed backup job", "job", job.Name, "succeeded", job.Status.Succeeded, "failed", job.Status.Failed)
			statusUpdated, err := m.processBackupJobResult(ctx, logger, cluster, job.Name)
			if err != nil {
				logger.Error(err, "Failed to process completed backup job", "job", job.Name)
				continue // Continue processing other jobs even if one fails
			}
			if statusUpdated {
				anyStatusUpdated = true
				logger.Info("Completed backup job processed, status updated", "job", job.Name)
				// Continue processing other jobs - we'll requeue after processing all
			} else {
				logger.V(1).Info("Completed backup job already processed", "job", job.Name)
			}
		}
	}

	return anyStatusUpdated, nil
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
		if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
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
