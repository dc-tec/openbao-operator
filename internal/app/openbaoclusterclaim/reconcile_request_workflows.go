package openbaoclusterclaim

import (
	"context"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (r runtimeReconciler) resolveActiveBackupRequest(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (*openbaov1alpha1.OpenBaoClusterClaimBackupRequest, error) {
	return listAndSelectClaimWorkflow(
		ctx,
		r.reader,
		claim,
		&openbaov1alpha1.OpenBaoClusterClaimBackupRequestList{},
		func(list *openbaov1alpha1.OpenBaoClusterClaimBackupRequestList) []openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
			return list.Items
		},
		func(candidate *openbaov1alpha1.OpenBaoClusterClaimBackupRequest, claim *openbaov1alpha1.OpenBaoClusterClaim) bool {
			return candidate.DeletionTimestamp.IsZero() &&
				candidate.Spec.ClaimRef.Name == claim.Name &&
				!isTerminalClaimBackupRequestState(candidate.Status.State)
		},
		claimBackupRequestIsEarlier,
	)
}

func (r runtimeReconciler) resolveActiveRestoreRequest(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (*openbaov1alpha1.OpenBaoClusterClaimRestoreRequest, error) {
	return listAndSelectClaimWorkflow(
		ctx,
		r.reader,
		claim,
		&openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList{},
		func(list *openbaov1alpha1.OpenBaoClusterClaimRestoreRequestList) []openbaov1alpha1.OpenBaoClusterClaimRestoreRequest {
			return list.Items
		},
		func(candidate *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest, claim *openbaov1alpha1.OpenBaoClusterClaim) bool {
			return candidate.DeletionTimestamp.IsZero() &&
				candidate.Spec.ClaimRef.Name == claim.Name &&
				!isTerminalClaimRestoreRequestState(candidate.Status.State)
		},
		claimRestoreRequestIsEarlier,
	)
}

func backupRequestSourceRef(request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) *openbaov1alpha1.TypedObjectReference {
	if request == nil {
		return nil
	}
	return workflowSourceRef(kindOpenBaoClusterClaimBackupRequest, request)
}

func claimRestoreRequestSourceRef(request *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) *openbaov1alpha1.TypedObjectReference {
	if request == nil {
		return nil
	}
	return workflowSourceRef(kindOpenBaoClusterClaimRestoreRequest, request)
}

func isTerminalClaimBackupRequestState(state openbaov1alpha1.OpenBaoClusterClaimBackupRequestState) bool {
	return stateIn(
		state,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed,
	)
}

func isTerminalClaimRestoreRequestState(state openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState) bool {
	return stateIn(
		state,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed,
	)
}

func claimBackupRequestIsEarlier(a, b *openbaov1alpha1.OpenBaoClusterClaimBackupRequest) bool {
	return workflowObjectIsEarlier(a, b)
}

func claimRestoreRequestIsEarlier(a, b *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest) bool {
	return workflowObjectIsEarlier(a, b)
}
