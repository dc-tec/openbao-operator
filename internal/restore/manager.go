// Package restore provides restore management for OpenBao clusters.
// It handles restoring snapshots from object storage to an OpenBao cluster.
package restore

import (
	"context"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
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
	client client.Client
	scheme *runtime.Scheme
}

// NewManager creates a new restore Manager.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: scheme,
	}
}

// Reconcile processes an OpenBaoRestore resource through its lifecycle.
// Returns (result, error) where result.Requeue or result.RequeueAfter
// indicates if reconciliation should be rescheduled.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (ctrl.Result, error) {
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
			return m.failRestore(ctx, restore, fmt.Sprintf("target cluster %q not found", restore.Spec.Cluster))
		}
		return ctrl.Result{}, fmt.Errorf("failed to get target cluster: %w", err)
	}

	// Check if cluster is in a valid state for restore (unless Force is set)
	if !restore.Spec.Force {
		// Check if cluster is initialized
		if !cluster.Status.Initialized {
			return m.failRestore(ctx, restore, "target cluster is not initialized (use force: true to override)")
		}

		// Check if cluster is upgrading
		upgradingCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
		if upgradingCond != nil && upgradingCond.Status == metav1.ConditionTrue {
			return m.failRestore(ctx, restore, "cannot restore while cluster is upgrading")
		}
	}

	// Check for existing restore in progress for this cluster
	restoreList := &openbaov1alpha1.OpenBaoRestoreList{}
	if err := m.client.List(ctx, restoreList, client.InNamespace(restore.Namespace)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list restores: %w", err)
	}

	for i := range restoreList.Items {
		other := &restoreList.Items[i]
		if other.Name == restore.Name {
			continue // Skip self
		}
		if other.Spec.Cluster == restore.Spec.Cluster &&
			(other.Status.Phase == openbaov1alpha1.RestorePhaseRunning ||
				other.Status.Phase == openbaov1alpha1.RestorePhaseValidating) {
			return m.failRestore(ctx, restore, fmt.Sprintf("another restore %q is already in progress for cluster %q", other.Name, restore.Spec.Cluster))
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
			return m.failRestore(ctx, restore, fmt.Sprintf("failed to build restore job: %v", err))
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
		_ = m.client.Status().Update(ctx, restore)

		// Requeue to check job status
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get restore job: %w", err)
	}

	// Check job status
	if job.Status.Succeeded > 0 {
		return m.completeRestore(ctx, restore, "Restore completed successfully")
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
		return m.failRestore(ctx, restore, message)
	}

	// Job still running
	restore.Status.Message = "Restore job in progress"
	_ = m.client.Status().Update(ctx, restore)

	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

// failRestore transitions the restore to Failed phase.
func (m *Manager) failRestore(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
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

	return ctrl.Result{}, nil
}

// completeRestore transitions the restore to Completed phase.
func (m *Manager) completeRestore(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
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

	return ctrl.Result{}, nil
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
