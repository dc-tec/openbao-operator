// Package restore provides restore management for OpenBao clusters.
// It handles restoring snapshots from object storage to an OpenBao cluster.
package restore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/internal/operationlock"
)

const (
	// RestoreJobNamePrefix is the prefix for restore job names.
	RestoreJobNamePrefix = constants.PrefixRestoreJob
	// RestoreJobTTLSeconds is the TTL for completed/failed restore jobs.
	RestoreJobTTLSeconds = 3600 // 1 hour
	// RestoreServiceAccountSuffix is appended to cluster name for the restore SA.
	RestoreServiceAccountSuffix = constants.SuffixRestoreServiceAccount
	// RestoreConditionType is the condition type for restore operations.
	RestoreConditionType = constants.RestoreConditionType // This will need to be added to conditions.go if missed
)

// Manager orchestrates restore operations for OpenBao clusters.
type Manager struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// NewManager creates a new restore Manager.
func NewManager(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder) *Manager {
	return &Manager{
		client:   c,
		scheme:   scheme,
		recorder: recorder,
	}
}

// Reconcile processes an OpenBaoRestore resource through its lifecycle.
// Returns (result, error) where result.Requeue or result.RequeueAfter
// indicates if reconciliation should be rescheduled.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	if restore.DeletionTimestamp != nil {
		return m.handleDeletion(ctx, logger, restore)
	}
	if err := m.ensureFinalizer(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure restore finalizer: %w", err)
	}

	// Initialize status if not set
	if restore.Status.Phase == "" {
		restore.Status.Phase = openbaov1alpha1.RestorePhasePending
	}

	switch restore.Status.Phase {
	case openbaov1alpha1.RestorePhasePending:
		return m.handlePending(ctx, logger, restore)
	case openbaov1alpha1.RestorePhaseValidating:
		return m.handleValidating(ctx, logger, restore)
	case openbaov1alpha1.RestorePhaseRunning:
		return m.handleRunning(ctx, logger, restore)
	case openbaov1alpha1.RestorePhaseCompleted, openbaov1alpha1.RestorePhaseFailed:
		// Terminal states - nothing to do
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown restore phase", "phase", restore.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// handlePending transitions from Pending to Validating phase.
func (m *Manager) handlePending(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	logger.Info("Starting restore validation", "cluster", restore.Spec.Cluster)

	// Record start time
	now := metav1.Now()
	restore.Status.StartTime = &now
	restore.Status.Phase = openbaov1alpha1.RestorePhaseValidating
	restore.Status.SnapshotKey = restore.Spec.Source.Key
	restore.Status.Message = "Validating restore preconditions"

	if err := m.client.Status().Update(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update restore status: %w", err)
	}

	return ctrl.Result{Requeue: true}, nil
}

// handleValidating validates preconditions and transitions to Running.
func (m *Manager) handleValidating(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	// Validate target cluster exists
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return m.failRestore(ctx, logger, restore, fmt.Sprintf("target cluster %q not found", restore.Spec.Cluster))
		}
		return ctrl.Result{}, fmt.Errorf("failed to get target cluster: %w", err)
	}

	lockHolder := fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name)
	lockMessage := fmt.Sprintf("restore %s/%s", restore.Namespace, restore.Name)
	forceAcquire := false

	if restore.Spec.OverrideOperationLock {
		if !restore.Spec.Force {
			return m.failRestore(ctx, logger, restore, "overrideOperationLock requires force: true")
		}
		if cluster.Status.OperationLock != nil && cluster.Status.OperationLock.Operation != openbaov1alpha1.ClusterOperationRestore {
			forceAcquire = true
		}
	}

	lockBefore := cluster.Status.OperationLock
	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    lockHolder,
		Operation: openbaov1alpha1.ClusterOperationRestore,
		Message:   lockMessage,
		Force:     forceAcquire,
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			var held *operationlock.HeldError
			if errors.As(err, &held) {
				restore.Status.Message = fmt.Sprintf("Waiting for cluster operation lock: operation=%s holder=%s", held.Operation, held.Holder)
			} else {
				restore.Status.Message = "Waiting for cluster operation lock"
			}
			if statusErr := m.client.Status().Update(ctx, restore); statusErr != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update restore status after lock contention: %w", statusErr)
			}
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to acquire cluster operation lock: %w", err)
	}

	if forceAcquire && lockBefore != nil {
		if m.recorder != nil {
			m.recorder.Eventf(restore, corev1.EventTypeWarning, "OperationLockOverride",
				"OverrideOperationLock used; cleared existing lock operation=%s holder=%s", lockBefore.Operation, lockBefore.Holder)
		}
		meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
			Type:               "OperationLockOverride",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "OperationLockOverridden",
			Message:            fmt.Sprintf("Cleared existing lock operation=%s holder=%s", lockBefore.Operation, lockBefore.Holder),
		})
	}

	// Check if cluster is in a valid state for restore (unless Force is set)
	if !restore.Spec.Force {
		// Check if cluster is initialized
		if !cluster.Status.Initialized {
			return m.failRestore(ctx, logger, restore, "target cluster is not initialized (use force: true to override)")
		}

		// Check if cluster is upgrading
		upgradingCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
		if upgradingCond != nil && upgradingCond.Status == metav1.ConditionTrue {
			return m.failRestore(ctx, logger, restore, "cannot restore while cluster is upgrading")
		}
	}

	// Ensure restore ServiceAccount exists
	if err := m.ensureRestoreServiceAccount(ctx, logger, restore, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure restore service account: %w", err)
	}

	// Ensure RBAC for restore
	if err := m.ensureRestoreRBAC(ctx, logger, restore, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure restore RBAC: %w", err)
	}

	// Transition to Running phase
	restore.Status.Phase = openbaov1alpha1.RestorePhaseRunning
	restore.Status.Message = "Creating restore job"

	if err := m.client.Status().Update(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update restore status: %w", err)
	}

	logger.Info("Restore validation passed, transitioning to Running phase")
	return ctrl.Result{Requeue: true}, nil
}

// handleRunning manages the restore job and checks for completion.
func (m *Manager) handleRunning(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	// Get target cluster for job configuration
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get target cluster: %w", err)
	}

	lockHolder := fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name)
	lockMessage := fmt.Sprintf("restore %s/%s", restore.Namespace, restore.Name)
	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    lockHolder,
		Operation: openbaov1alpha1.ClusterOperationRestore,
		Message:   lockMessage,
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			return m.failRestore(ctx, logger, restore, "cluster operation lock was taken by another operation while restore was running")
		}
		return ctrl.Result{}, fmt.Errorf("failed to renew cluster operation lock: %w", err)
	}

	// Check if job already exists
	jobName := restoreJobName(restore)
	job := &batchv1.Job{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      jobName,
	}, job)

	if apierrors.IsNotFound(err) {
		// Create the restore job
		job, err = m.buildRestoreJob(restore, cluster)
		if err != nil {
			return m.failRestore(ctx, logger, restore, fmt.Sprintf("failed to build restore job: %v", err))
		}

		// Set owner reference
		if err := controllerutil.SetControllerReference(restore, job, m.scheme); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
		}

		if err := m.client.Create(ctx, job); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create restore job: %w", err)
		}

		logger.Info("Created restore job", "job", jobName)
		restore.Status.Message = "Restore job running"
		if err := m.client.Status().Update(ctx, restore); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update restore status after job creation: %w", err)
		}

		// Requeue to check job status
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get restore job: %w", err)
	}

	// Check job status
	if job.Status.Succeeded > 0 {
		return m.completeRestore(ctx, logger, restore, "Restore completed successfully")
	}

	if job.Status.Failed > 0 {
		// Get failure message from job conditions
		message := "Restore job failed"
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				if cond.Message != "" {
					message = fmt.Sprintf("Restore job failed: %s", cond.Message)
				}
				break
			}
		}
		return m.failRestore(ctx, logger, restore, message)
	}

	// Job still running
	restore.Status.Message = "Restore job in progress"
	if err := m.client.Status().Update(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update restore status while job is running: %w", err)
	}

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// failRestore transitions the restore to Failed phase.
func (m *Manager) failRestore(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
	now := metav1.Now()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseFailed
	restore.Status.CompletionTime = &now
	restore.Status.Message = message

	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionFalse,
		Reason:             ReasonRestoreFailed,
		Message:            message,
		LastTransitionTime: now,
	})

	if err := m.client.Status().Update(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update restore status: %w", err)
	}

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock after restore failure")
	}

	return ctrl.Result{}, nil
}

// completeRestore transitions the restore to Completed phase.
func (m *Manager) completeRestore(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
	now := metav1.Now()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseCompleted
	restore.Status.CompletionTime = &now
	restore.Status.Message = message

	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionTrue,
		Reason:             ReasonRestoreSucceeded,
		Message:            message,
		LastTransitionTime: now,
	})

	if err := m.client.Status().Update(ctx, restore); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update restore status: %w", err)
	}

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock after restore completion")
	}

	return ctrl.Result{}, nil
}

func (m *Manager) ensureFinalizer(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore) error {
	if controllerutil.ContainsFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer) {
		return nil
	}

	original := restore.DeepCopy()
	controllerutil.AddFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer)
	if err := m.client.Patch(ctx, restore, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to add finalizer: %w", err)
	}
	return nil
}

func (m *Manager) handleDeletion(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer) {
		return ctrl.Result{}, nil
	}

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock during restore deletion")
	}

	original := restore.DeepCopy()
	controllerutil.RemoveFinalizer(restore, openbaov1alpha1.OpenBaoRestoreFinalizer)
	if err := m.client.Patch(ctx, restore, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	return ctrl.Result{}, nil
}

func (m *Manager) releaseClusterLock(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) error {
	if restore.Spec.Cluster == "" {
		return nil
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get target cluster for lock release: %w", err)
	}

	holder := fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name)
	if err := operationlock.Release(ctx, m.client, cluster, holder, openbaov1alpha1.ClusterOperationRestore); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			return nil
		}
		return err
	}

	logger.V(1).Info("Released cluster operation lock for restore", "cluster", cluster.Name)
	return nil
}

// restoreJobName returns the name for the restore job.
func restoreJobName(restore *openbaov1alpha1.OpenBaoRestore) string {
	return fmt.Sprintf("%s%s", RestoreJobNamePrefix, restore.Name)
}

// restoreServiceAccountName returns the name for the restore ServiceAccount.
func restoreServiceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + RestoreServiceAccountSuffix
}

// ensureRestoreServiceAccount creates the ServiceAccount for restore jobs.
func (m *Manager) ensureRestoreServiceAccount(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := restoreServiceAccountName(cluster)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: restore.Namespace,
			Labels:    restoreLabels(cluster),
		},
	}

	// Set owner reference to the cluster (not the restore) so SA persists
	if err := controllerutil.SetControllerReference(cluster, sa, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ServiceAccount{}
	err := m.client.Get(ctx, types.NamespacedName{Name: saName, Namespace: restore.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := m.client.Create(ctx, sa); err != nil {
			return fmt.Errorf("failed to create restore service account: %w", err)
		}
		logger.Info("Created restore service account", "name", saName)
	} else if err != nil {
		return fmt.Errorf("failed to get restore service account: %w", err)
	}

	return nil
}

// ensureRestoreRBAC creates RBAC for the restore service account.
func (m *Manager) ensureRestoreRBAC(ctx context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoRestore, _ *openbaov1alpha1.OpenBaoCluster) error {
	// For now, we rely on the existing RBAC from the backup setup
	// The restore SA needs permission to list pods (to find leader)
	// This can be expanded later if needed
	return nil
}

// restoreLabels returns standard labels for restore resources.
func restoreLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentRestore,
	}
}
