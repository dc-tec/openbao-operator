package restore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	observability "github.com/dc-tec/openbao-operator/internal/platform/observability"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

func (m *Manager) patchStatus(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, original *openbaov1alpha1.OpenBaoRestore) error {
	return m.client.Status().Patch(ctx, restore, client.MergeFrom(original))
}

// failRestore transitions the restore to Failed phase.
//
//nolint:unparam // ctrl.Result is always zero value but required by controller-runtime interface
func (m *Manager) failRestore(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseFailed
	restore.Status.CompletionTime = &now
	restore.Status.Message = message

	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: restore.Generation,
		Reason:             ReasonRestoreFailed,
		Message:            message,
		LastTransitionTime: now,
	})

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch restore status: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventRestoreFailed, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
	})
	m.emitWarningEvent(restore, ReasonRestoreFailed, "%s", message)

	durationSeconds := 0.0
	if restore.Status.StartTime != nil {
		durationSeconds = now.Time.Sub(restore.Status.StartTime.Time).Seconds()
	}
	observability.NewRestoreMetrics(restore.Namespace, restore.Spec.Cluster).RecordFailureWithDuration(durationSeconds)

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock after restore failure")
	}

	return ctrl.Result{}, nil
}

func (m *Manager) patchRestoreConfigurationCondition(
	ctx context.Context,
	restore *openbaov1alpha1.OpenBaoRestore,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	current := meta.FindStatusCondition(restore.Status.Conditions, RestoreConfigurationConditionType)
	if current != nil &&
		current.Status == status &&
		current.Reason == reason &&
		current.Message == message &&
		current.ObservedGeneration == restore.Generation {
		return nil
	}

	original := restore.DeepCopy()
	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               RestoreConfigurationConditionType,
		Status:             status,
		ObservedGeneration: restore.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return fmt.Errorf("failed to patch restore configuration status: %w", err)
	}

	return nil
}

// completeRestore transitions the restore to Completed phase.
func (m *Manager) completeRestore(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore, message string) error {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseCompleted
	restore.Status.CompletionTime = &now
	restore.Status.Message = message

	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: restore.Generation,
		Reason:             ReasonRestoreSucceeded,
		Message:            message,
		LastTransitionTime: now,
	})

	if err := m.patchStatus(ctx, restore, original); err != nil {
		return fmt.Errorf("failed to patch restore status: %w", err)
	}
	logging.LogAuditEvent(logger, logging.EventRestoreCompleted, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
	})
	m.emitNormalEvent(restore, ReasonRestoreCompleted, "%s", message)

	durationSeconds := 0.0
	if restore.Status.StartTime != nil {
		durationSeconds = now.Time.Sub(restore.Status.StartTime.Time).Seconds()
	}
	observability.NewRestoreMetrics(restore.Namespace, restore.Spec.Cluster).RecordSuccess(durationSeconds)

	if err := m.releaseClusterLock(ctx, logger, restore); err != nil {
		logger.Error(err, "Failed to release cluster operation lock after restore completion")
	}

	return nil
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
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(restore), restore); err != nil {
		return fmt.Errorf("failed to refresh restore after adding finalizer: %w", err)
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
	if err := m.reader.Get(ctx, types.NamespacedName{
		Namespace: restore.Namespace,
		Name:      restore.Spec.Cluster,
	}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get target cluster for lock release: %w", err)
	}

	lock := restoreOperationLock(restore)
	if err := opslifecycle.ReleaseWithReader(ctx, m.reader, m.client, cluster, lock); err != nil {
		if opslifecycle.IsLockHeld(err) {
			return nil
		}
		return err
	}
	logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
		"cluster_namespace": restore.Namespace,
		"cluster_name":      restore.Spec.Cluster,
		"restore_name":      restore.Name,
		"operation":         string(openbaov1alpha1.ClusterOperationRestore),
		"holder":            lock.Holder,
	})

	logger.V(1).Info("Released cluster operation lock for restore", "cluster", cluster.Name)
	return nil
}
