package statusops

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
)

func TestPolicyReconciliationCondition(t *testing.T) {
	for _, tc := range []struct {
		name, blocked, reason string
		status                metav1.ConditionStatus
		change                func(*openbaov1alpha1.OpenBaoCluster)
	}{
		{name: "verified", status: metav1.ConditionTrue, reason: "PoliciesVerified"},
		{name: "new revision", status: metav1.ConditionFalse, reason: "PolicyVerificationPending", change: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
		}},
		{name: "not enrolled", status: metav1.ConditionFalse, reason: "PolicyVerificationPending", change: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.Workload = nil
		}},
		{name: "not initialized", status: metav1.ConditionFalse, reason: "WaitingForInitialization", change: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.Initialized = false
		}},
		{name: "failed", status: metav1.ConditionFalse, reason: "PolicyReconciliationFailed", change: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.Workload.PolicyReconciliation.LastError = &openbaov1alpha1.ControllerErrorStatus{Reason: "PolicyReconciliationFailed", Message: "Complete administrator enrollment"}
		}},
		{name: "paused", blocked: reasonPaused, status: metav1.ConditionUnknown, reason: reasonPaused},
		{name: "disabled", change: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.ReconcilePolicies = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			cluster.Generation = 7
			cluster.Spec.ReconcilePolicies, cluster.Status.Initialized = true, true
			now := metav1.Now()
			cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{
				PolicyRevision:       configuration.PolicyRevision(cluster),
				PolicyReconciliation: &openbaov1alpha1.PolicyReconciliationStatus{LastVerified: &now},
			}
			applyPolicyReconciliationCondition(cluster, now, "")
			if tc.change != nil {
				tc.change(cluster)
			}
			applyPolicyReconciliationCondition(cluster, now, tc.blocked)
			condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionPolicyReconciliationReady))
			if tc.name == "disabled" {
				require.Nil(t, condition)
				return
			}
			require.NotNil(t, condition)
			require.Equal(t, tc.status, condition.Status)
			require.Equal(t, tc.reason, condition.Reason)
			require.Equal(t, cluster.Generation, condition.ObservedGeneration)
			require.False(t, condition.LastTransitionTime.IsZero())
			if tc.name == "failed" {
				require.Equal(t, "PolicyReconciliationFailed", buildDegradedCondition(cluster, false).Reason)
			}
		})
	}
}
