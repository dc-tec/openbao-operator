//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
)

type policyWriterFunc func(context.Context, string, string) error

func (f policyWriterFunc) WriteACLPolicy(ctx context.Context, name, policy string) error {
	return f(ctx, name, policy)
}

type policyRepairObserver struct {
	observe func(*openbaov1alpha1.OpenBaoCluster)
}

func (r policyRepairObserver) Reconcile(_ context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	r.observe(cluster)
	return recon.Result{RequeueAfter: time.Second}, nil
}

func TestPolicyReconciliationStatus(t *testing.T) {
	cluster := newMinimalClusterObj(newTestNamespace(t), "policy-reconciliation")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true, ReconcilePolicies: true}}
	require.NoError(t, k8sClient.Create(ctx, cluster))
	cluster.Status.Initialized = true
	require.NoError(t, k8sClient.Status().Update(ctx, cluster))
	denied := false
	manager := &configuration.PolicyManager{ClientFor: func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyWriter, error) {
		return policyWriterFunc(func(context.Context, string, string) error {
			if denied {
				return fmt.Errorf("approval denied")
			}
			return nil
		}), nil
	}}
	observations := 0
	applications := openbaocluster.NewApplications(openbaocluster.ApplicationsConfig{
		Client:           k8sClient,
		PolicyReconciler: manager,
		WorkloadPolicy:   openbaocluster.DefaultWorkloadResultPolicy(),
		WorkloadReconcilers: []openbaocluster.SubReconciler{policyRepairObserver{observe: func(c *openbaov1alpha1.OpenBaoCluster) {
			observations++
			if !denied {
				require.NoError(t, configuration.RequirePoliciesReady(c), "repair precedes infrastructure operations")
			}
		}}},
	})
	for _, fail := range []bool{false, true, false} {
		denied = fail
		result, err := applications.ReconcileWorkload(ctx, logr.Discard(), cluster.DeepCopy(), cluster, nil)
		require.NoError(t, err)
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
		if fail {
			require.Positive(t, result.RequeueAfter)
			require.Empty(t, cluster.Status.Workload.PolicyRevision, "SSA must remove the earlier successful revision")
			require.Equal(t, "PolicyReconciliationFailed", cluster.Status.Workload.LastError.Reason)
			require.Error(t, configuration.RequirePoliciesReady(cluster))
		} else {
			require.Nil(t, cluster.Status.Workload.LastError)
			require.NoError(t, configuration.RequirePoliciesReady(cluster))
		}
	}
	require.Equal(t, 3, observations, "infrastructure repair still runs when OpenBao policy writes fail")
}

func TestPolicyReconciliationRequiresJWTBootstrap(t *testing.T) {
	for _, requested := range []bool{false, true} {
		for _, selfInitEnabled := range []bool{false, true} {
			cluster := newMinimalClusterObj(newTestNamespace(t), "invalid-policy-reconciliation")
			cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: selfInitEnabled, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: !selfInitEnabled, ReconcilePolicies: requested}}
			err := k8sClient.Create(ctx, cluster)
			if !requested {
				require.NoError(t, err)
				continue
			}
			requireInvalidRequest(t, err)
			require.ErrorContains(t, err, "policy reconciliation requires")
		}
	}
}
