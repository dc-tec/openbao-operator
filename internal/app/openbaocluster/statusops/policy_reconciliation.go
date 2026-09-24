package statusops

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
)

const (
	reasonPolicyVerificationPending   = "PolicyVerificationPending"
	reasonPolicyInitializationPending = "WaitingForInitialization"
	reasonPoliciesVerified            = "PoliciesVerified"
)

func applyPolicyReconciliationCondition(cluster *openbaov1alpha1.OpenBaoCluster, now metav1.Time, blockedReason string) {
	conditionType := string(openbaov1alpha1.ConditionPolicyReconciliationReady)
	if !portauth.PolicyReconciliationEnabled(cluster) {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, conditionType)
		return
	}
	condition := metav1.Condition{
		Type: conditionType, Status: metav1.ConditionFalse,
		Reason: reasonPolicyVerificationPending, Message: "Waiting to verify built-in policies; complete administrator enrollment if required",
		ObservedGeneration: cluster.Generation, LastTransitionTime: now,
	}
	workload := cluster.Status.Workload
	switch {
	case blockedReason != "":
		condition.Status, condition.Reason = metav1.ConditionUnknown, blockedReason
		condition.Message = "Policy reconciliation readiness is not being evaluated while reconciliation is blocked"
	case !cluster.Status.Initialized:
		condition.Reason, condition.Message = reasonPolicyInitializationPending, "Policy verification starts after OpenBao initialization"
	case workload != nil && workload.PolicyReconciliation != nil:
		status := workload.PolicyReconciliation
		if status.LastError != nil {
			condition.Reason, condition.Message = status.LastError.Reason, status.LastError.Message
		} else if status.LastVerified != nil && workload.PolicyRevision == configuration.PolicyRevision(cluster) {
			condition.Status, condition.Reason = metav1.ConditionTrue, reasonPoliciesVerified
			condition.Message = "The requested built-in policies were verified successfully; see status.workload.policyReconciliation.lastVerified"
		}
	}
	meta.SetStatusCondition(&cluster.Status.Conditions, condition)
}
