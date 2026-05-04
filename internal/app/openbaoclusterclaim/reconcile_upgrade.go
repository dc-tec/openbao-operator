package openbaoclusterclaim

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (r runtimeReconciler) resolveActiveUpgradeRequest(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (*openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest, error) {
	if claim == nil || claim.Namespace == "" || claim.Name == "" {
		return nil, nil
	}

	list := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestList{}
	if err := r.reader.List(ctx, list, client.InNamespace(claim.Namespace)); err != nil {
		return nil, err
	}

	var active *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest
	for i := range list.Items {
		candidate := &list.Items[i]
		if !candidate.DeletionTimestamp.IsZero() ||
			candidate.Spec.ClaimRef.Name != claim.Name ||
			isTerminalClaimUpgradeRequestState(candidate.Status.State) {
			continue
		}
		if active == nil || claimUpgradeRequestIsEarlier(candidate, active) {
			active = candidate
		}
	}
	return active, nil
}

func desiredUpgradeStatus(
	request *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
) *openbaov1alpha1.OpenBaoClusterClaimUpgradeStatus {
	if request == nil {
		return nil
	}

	state := request.Status.State
	if state == "" {
		state = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending
	}

	status := &openbaov1alpha1.OpenBaoClusterClaimUpgradeStatus{
		RequestRef: &openbaov1alpha1.LocalReference{Name: request.Name},
		State:      state,
		Reason:     request.Status.Reason,
	}
	if request.Status.Classification != nil {
		classification := *request.Status.Classification
		status.Classification = &classification
	}
	return status
}

func isTerminalClaimUpgradeRequestState(state openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestState) bool {
	switch state {
	case openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateSucceeded,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateBlocked,
		openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateFailed:
		return true
	default:
		return false
	}
}

func claimUpgradeRequestIsEarlier(a, b *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest) bool {
	if a == nil || b == nil {
		return false
	}
	if a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.Name < b.Name
	}
	return a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time)
}
