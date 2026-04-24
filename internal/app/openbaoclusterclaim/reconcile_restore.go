package openbaoclusterclaim

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (r runtimeReconciler) resolveActiveRestoreExecution(
	ctx context.Context,
	localTarget *openbaov1alpha1.NamespacedReference,
	localResolved result,
) (*openbaov1alpha1.OpenBaoRestore, error) {
	if !localResolved.Valid || localTarget == nil || localTarget.Namespace == "" || localTarget.Name == "" {
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoRestoreList{}
	if err := r.reader.List(ctx, list, client.InNamespace(localTarget.Namespace)); err != nil {
		return nil, err
	}

	var active *openbaov1alpha1.OpenBaoRestore
	for i := range list.Items {
		candidate := &list.Items[i]
		if !candidate.DeletionTimestamp.IsZero() ||
			candidate.Spec.Cluster != localTarget.Name ||
			isTerminalRestorePhase(candidate.Status.Phase) {
			continue
		}
		if active == nil || restoreIsEarlier(candidate, active) {
			active = candidate
		}
	}
	return active, nil
}

func desiredRestoreStatus(
	request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest,
	restore *openbaov1alpha1.OpenBaoRestore,
) *openbaov1alpha1.OpenBaoClusterClaimRestoreStatus {
	if request == nil && restore == nil {
		return nil
	}

	status := &openbaov1alpha1.OpenBaoClusterClaimRestoreStatus{}
	if request != nil {
		state := request.Status.State
		if state == "" {
			state = openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending
		}
		status.RequestRef = &openbaov1alpha1.LocalReference{Name: request.Name}
		status.RequestState = state
		status.RequestReason = request.Status.Reason
		status.ExecutionRef = request.Status.RestoreRef
		status.SnapshotKey = request.Status.SnapshotKey
		if request.Status.StartTime != nil {
			status.StartTime = request.Status.StartTime.DeepCopy()
		}
	}
	if restore != nil {
		state := restore.Status.Phase
		if state == "" {
			state = openbaov1alpha1.RestorePhasePending
		}
		snapshotKey := restore.Status.SnapshotKey
		if snapshotKey == "" {
			snapshotKey = restore.Spec.Source.Key
		}
		status.ExecutionRef = &openbaov1alpha1.NamespacedReference{Namespace: restore.Namespace, Name: restore.Name}
		status.State = state
		status.SnapshotKey = snapshotKey
		if restore.Status.StartTime != nil {
			status.StartTime = restore.Status.StartTime.DeepCopy()
		}
		status.Message = restore.Status.Message
	}

	if status.RequestRef != nil ||
		status.ExecutionRef != nil ||
		status.RequestState != "" ||
		status.State != "" ||
		status.SnapshotKey != "" ||
		status.StartTime != nil ||
		status.Message != "" {
		return status
	}
	return nil
}

func isTerminalRestorePhase(phase openbaov1alpha1.RestorePhase) bool {
	switch phase {
	case openbaov1alpha1.RestorePhaseCompleted, openbaov1alpha1.RestorePhaseFailed:
		return true
	default:
		return false
	}
}

func restoreIsEarlier(a, b *openbaov1alpha1.OpenBaoRestore) bool {
	if a == nil || b == nil {
		return false
	}
	if a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.Name < b.Name
	}
	return a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time)
}
