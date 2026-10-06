package restore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
)

func (m *Manager) cleanupTarget(ctx context.Context, logger logr.Logger, request *api.OpenBaoRestore) (ctrl.Result, error) {
	target := request.Status.Target
	if target == nil {
		return ctrl.Result{}, nil
	}
	if target.Cleanup == api.RestoreTargetCleanupComplete {
		return m.finishTargetCleanup(ctx, logger, request)
	}
	if request.Status.AdministratorDisposition == api.RestoreAdministratorAbandon {
		return ctrl.Result{}, nil
	}
	if target.Cleanup == "" {
		before := request.DeepCopy()
		target.Cleanup = api.RestoreTargetCleanupPending
		return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, m.patchStatus(ctx, request, before)
	}
	if target.Cleanup == api.RestoreTargetCleanupFailed {
		result, err := m.reconcileAcknowledgement(ctx, logger, request)
		if err != nil || request.Status.AdministratorDisposition != "" {
			return result, err
		}
		// A refused PVC must not keep the bound disposable cluster running.
		// Continue cluster deletion, but retain the failed PVC cleanup outcome.
	}
	fail := func(message string) (ctrl.Result, error) {
		if target.Cleanup == api.RestoreTargetCleanupFailed && request.Status.Message == message {
			return ctrl.Result{}, nil
		}
		before := request.DeepCopy()
		target.Cleanup = api.RestoreTargetCleanupFailed
		request.Status.Message = message
		meta.SetStatusCondition(&request.Status.Conditions, metav1.Condition{
			Type: constants.RestoreRecoveryReleasedConditionType, Status: metav1.ConditionFalse,
			Reason: ReasonRecoveryBlocked, Message: message, ObservedGeneration: request.Generation,
		})
		return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, m.patchStatus(ctx, request, before)
	}
	if request.Status.Execution != nil {
		done, err := m.deleteRestoreJob(ctx, logger, request)
		if err != nil || !done {
			return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, err
		}
	}

	cluster := &api.OpenBaoCluster{}
	err := m.reader.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: request.Spec.Cluster}, cluster)
	if err == nil {
		return m.cleanupTargetCluster(ctx, logger, request, cluster, fail)
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if target.Cleanup == api.RestoreTargetCleanupFailed {
		return ctrl.Result{}, nil
	}
	if target.UID == "" {
		// No target was bound and none exists with our origin. An existing PVC
		// belongs to another actor, so it is outside this request's cleanup.
		return m.markTargetCleanupComplete(ctx, logger, request)
	}

	return m.cleanupTargetPVC(ctx, logger, request, fail)
}

func (m *Manager) cleanupTargetCluster(ctx context.Context, logger logr.Logger, request *api.OpenBaoRestore,
	cluster *api.OpenBaoCluster, fail func(string) (ctrl.Result, error),
) (ctrl.Result, error) {
	target := request.Status.Target
	if target.UID == "" && cluster.Annotations[constants.AnnotationRestoreOrigin] != string(request.UID) {
		// A rejected creation never owned this cluster or its data volume.
		return m.markTargetCleanupComplete(ctx, logger, request)
	}
	if target.UID == "" {
		before := request.DeepCopy()
		target.UID = cluster.UID
		return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, m.patchStatus(ctx, request, before)
	}
	if cluster.UID != target.UID || cluster.Annotations[constants.AnnotationRestoreOrigin] != string(request.UID) {
		return fail("Cleanup refused: target identity is unbound or replaced")
	}
	if target.DataPVCUID == "" && target.Cleanup != api.RestoreTargetCleanupFailed {
		pvc := &corev1.PersistentVolumeClaim{}
		err := m.reader.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: targetPVCName(request)}, pvc)
		if err == nil {
			if !originalTargetPVC(pvc, cluster) {
				return fail("Cleanup refused: data PVC predates the target or lacks original owner proof")
			}
			before := request.DeepCopy()
			target.DataPVCUID = pvc.UID
			return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, m.patchStatus(ctx, request, before)
		}
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if cluster.Annotations[constants.AnnotationRestoreHold] != "" {
		if cluster.Annotations[constants.AnnotationRestoreHold] != string(request.UID) {
			return fail("Cleanup refused: target has another restore hold")
		}
		before := cluster.DeepCopy()
		cluster.Spec.Paused = true
		delete(cluster.Annotations, constants.AnnotationRestoreHold)
		return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, m.client.Patch(ctx, cluster, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{}))
	}
	if cluster.DeletionTimestamp == nil {
		if err := m.client.Delete(ctx, cluster, client.Preconditions{UID: &target.UID}, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, nil
}

func (m *Manager) cleanupTargetPVC(ctx context.Context, logger logr.Logger, request *api.OpenBaoRestore,
	fail func(string) (ctrl.Result, error),
) (ctrl.Result, error) {
	target := request.Status.Target
	pvc := &corev1.PersistentVolumeClaim{}
	err := m.reader.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: targetPVCName(request)}, pvc)
	if err == nil {
		// A StatefulSet can create its retained PVC during foreground deletion.
		// Its owner annotation identifies the original cluster. This assumes trusted
		// destination workloads: tenants can copy it through their own StatefulSet.
		// It prevents accidental adoption, not forgery by a destination tenant.
		if target.DataPVCUID == "" && resourceownership.HasOwnerUIDAnnotation(pvc,
			&api.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{UID: target.UID}}) {
			before := request.DeepCopy()
			target.DataPVCUID = pvc.UID
			return ctrl.Result{RequeueAfter: restoreRequeueImmediately}, m.patchStatus(ctx, request, before)
		}
		if target.DataPVCUID == "" || pvc.UID != target.DataPVCUID {
			return fail("Cleanup refused: data PVC identity is unbound or replaced")
		}
		if err := m.client.Delete(ctx, pvc, client.Preconditions{UID: &target.DataPVCUID}); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("delete fresh target PVC: %w", err)
		}
		return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, nil
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return m.markTargetCleanupComplete(ctx, logger, request)
}

func (m *Manager) markTargetCleanupComplete(ctx context.Context, logger logr.Logger, request *api.OpenBaoRestore) (ctrl.Result, error) {
	before := request.DeepCopy()
	request.Status.Target.Cleanup = api.RestoreTargetCleanupComplete
	if err := m.patchStatus(ctx, request, before); err != nil {
		return ctrl.Result{}, err
	}
	return m.finishTargetCleanup(ctx, logger, request)
}

// Re-entry finishes a cleanup whose status patch succeeded before a controller restart.
func (m *Manager) finishTargetCleanup(ctx context.Context, logger logr.Logger, request *api.OpenBaoRestore) (ctrl.Result, error) {
	if request.Status.Phase == api.RestorePhaseFailed || request.Status.Phase == api.RestorePhaseCompleted {
		return ctrl.Result{}, nil
	}

	target := request.Status.Target
	if target.AppliedAt != nil {
		return ctrl.Result{}, m.completeRestore(ctx, logger, request, "Fresh target application confirmed; disposable Kubernetes resources deleted")
	}
	return m.failRestore(ctx, logger, request, "Disposable target deleted without confirmed snapshot application")
}
