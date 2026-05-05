package openbaoclusterclaim

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	reasonClaimAccepted     = "ClaimAccepted"
	reasonClaimMaterialized = "ClaimMaterialized"
	reasonClaimReady        = "ClaimReady"
	reasonClaimDegraded     = "ClaimDegraded"
	reasonClaimFailed       = "ClaimFailed"
	reasonClaimDeleting     = "ClaimDeleting"
)

func (r runtimeReconciler) emitClaimEvents(
	original *openbaov1alpha1.OpenBaoClusterClaim,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) {
	if r.recorder == nil || original == nil || claim == nil {
		return
	}

	if conditionTransitionedTrue(original.Status.Conditions, claim.Status.Conditions, conditionTypeAccepted) {
		condition := meta.FindStatusCondition(claim.Status.Conditions, conditionTypeAccepted)
		r.emitClaimEvent(claim, corev1.EventTypeNormal, reasonClaimAccepted, conditionMessageOrDefault(condition, "OpenBaoClusterClaim was accepted."))
	}
	if conditionTransitionedTrue(original.Status.Conditions, claim.Status.Conditions, conditionTypeMaterialization) &&
		claim.Status.Materialization.LocalRef != nil {
		ref := claim.Status.Materialization.LocalRef
		r.emitClaimEvent(
			claim,
			corev1.EventTypeNormal,
			reasonClaimMaterialized,
			fmt.Sprintf("OpenBaoClusterClaim materialized OpenBaoCluster %s/%s.", ref.Namespace, ref.Name),
		)
	}
	if original.Status.Phase == claim.Status.Phase {
		return
	}
	switch claim.Status.Phase {
	case openbaov1alpha1.OpenBaoClusterClaimPhaseReady:
		r.emitClaimEvent(claim, corev1.EventTypeNormal, reasonClaimReady, "OpenBaoClusterClaim is ready.")
	case openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded:
		r.emitClaimEvent(claim, corev1.EventTypeWarning, reasonClaimDegraded, claimPhaseEventMessage(claim, "OpenBaoClusterClaim is degraded."))
	case openbaov1alpha1.OpenBaoClusterClaimPhaseFailed:
		r.emitClaimEvent(claim, corev1.EventTypeWarning, reasonClaimFailed, claimPhaseEventMessage(claim, "OpenBaoClusterClaim failed."))
	case openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting:
		r.emitClaimEvent(claim, corev1.EventTypeNormal, reasonClaimDeleting, "OpenBaoClusterClaim deletion is in progress.")
	}
}

func (r runtimeReconciler) emitClaimEvent(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	eventType string,
	reason string,
	note string,
) {
	if r.recorder == nil || claim == nil || reason == "" || note == "" {
		return
	}
	r.recorder.Eventf(claim, nil, eventType, reason, reason, "%s", note)
}

func conditionTransitionedTrue(
	oldConditions []metav1.Condition,
	newConditions []metav1.Condition,
	conditionType string,
) bool {
	oldCondition := meta.FindStatusCondition(oldConditions, conditionType)
	newCondition := meta.FindStatusCondition(newConditions, conditionType)
	return newCondition != nil &&
		newCondition.Status == metav1.ConditionTrue &&
		(oldCondition == nil || oldCondition.Status != metav1.ConditionTrue || oldCondition.ObservedGeneration != newCondition.ObservedGeneration)
}

func conditionMessageOrDefault(condition *metav1.Condition, fallback string) string {
	if condition != nil && condition.Message != "" {
		return condition.Message
	}
	return fallback
}

func claimPhaseEventMessage(claim *openbaov1alpha1.OpenBaoClusterClaim, fallback string) string {
	if claim.Status.Summary == nil {
		return fallback
	}
	if claim.Status.Summary.Message != "" && claim.Status.Summary.Reason != "" {
		return fmt.Sprintf("%s: %s", claim.Status.Summary.Reason, claim.Status.Summary.Message)
	}
	if claim.Status.Summary.Message != "" {
		return claim.Status.Summary.Message
	}
	if claim.Status.Summary.Reason != "" {
		return claim.Status.Summary.Reason
	}
	return fallback
}
