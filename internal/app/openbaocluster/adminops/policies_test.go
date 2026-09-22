package adminops

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func TestPolicyGateBeforeNewOperations(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true, ReconcilePolicies: true}}
	plan := reconcilerPlan{upgradeReconcilers: []subReconciler{fakeSubReconciler{}}}
	reconcilers := plan.orderedFor(cluster)
	require.Len(t, reconcilers, 1)
	_, err := reconcilers[0].Reconcile(t.Context(), logr.Discard(), cluster)
	require.Error(t, err)
	cluster.Status.Workload = &openbaov1alpha1.WorkloadControllerStatus{PolicyRevision: configuration.PolicyRevision(cluster)}
	_, err = reconcilers[0].Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	cluster.Status.Workload.PolicyRevision = ""
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{Operation: openbaov1alpha1.ClusterOperationUpgrade}
	_, err = reconcilers[0].Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err, "running operations must observe completion and release their lock")
	plan.upgradeReconcilers = []subReconciler{
		fakeMutatingSubReconciler{mutate: func(c *openbaov1alpha1.OpenBaoCluster) { c.Status.OperationLock = nil }},
		fakeSubReconciler{},
	}
	reconcilers = plan.orderedFor(cluster)
	_, err = reconcilers[0].Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	_, err = reconcilers[1].Reconcile(t.Context(), logr.Discard(), cluster)
	require.Error(t, err, "a new operation cannot start after the previous operation releases its lock")
}
