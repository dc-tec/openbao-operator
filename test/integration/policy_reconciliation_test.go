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
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/configuration"
)

type policyClient struct {
	values      map[string]string
	readDenied  bool
	writeDenied bool
	writes      int
}

func (c *policyClient) ReadACLPolicy(_ context.Context, name string) (*string, error) {
	if c.readDenied {
		return nil, fmt.Errorf("read denied")
	}
	value, exists := c.values[name]
	if !exists {
		return nil, nil
	}
	return &value, nil
}

func (c *policyClient) WriteACLPolicy(_ context.Context, name, policy string) error {
	if c.writeDenied {
		return fmt.Errorf("approval denied")
	}
	c.values[name] = policy
	c.writes++
	return nil
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
	waitForOpenBaoClusterAdmissionPolicies(t, cluster.Namespace)
	cluster.Spec.ReconcilePolicies = true
	require.NoError(t, k8sClient.Create(ctx, cluster))
	cluster.Status.Initialized = true
	require.NoError(t, k8sClient.Status().Update(ctx, cluster))
	store := &policyClient{values: map[string]string{}}
	manager := &configuration.PolicyManager{ClientFor: func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyClient, error) {
		return store, nil
	}}
	observations := 0
	applications := openbaocluster.NewApplications(openbaocluster.ApplicationsConfig{
		Client:           k8sClient,
		PolicyReconciler: manager,
		WorkloadPolicy:   openbaocluster.DefaultWorkloadResultPolicy(),
		WorkloadReconcilers: []openbaocluster.SubReconciler{policyRepairObserver{observe: func(c *openbaov1alpha1.OpenBaoCluster) {
			observations++
			if observations == 1 {
				require.NoError(t, configuration.RequirePolicyReady(c, portauth.PolicyNameOperator), "unobserved policies do not block infrastructure")
			}
		}}},
	})
	for _, step := range []struct {
		readDenied   bool
		writeDenied  bool
		deletePolicy bool
		writes       int
	}{
		{readDenied: true}, // Enabling before enrollment preserves existing operations.
		{writes: 3},
		{}, // A matching revision still reads live policies without rewriting them.
		{readDenied: true},
		{},
		{deletePolicy: true, writeDenied: true},
		{writes: 1},
	} {
		if cluster.Status.Workload != nil && cluster.Status.Workload.PolicyReconciliation != nil {
			cluster.Status.Workload.PolicyReconciliation.RetryAfter = nil
			cluster.Status.Workload.PolicyReconciliation.LastVerified = nil
		}
		store.readDenied, store.writeDenied = step.readDenied, step.writeDenied
		store.writes = 0
		if step.deletePolicy {
			delete(store.values, portauth.PolicyNameOperator)
		}
		hadVerified := cluster.Status.Workload != nil && cluster.Status.Workload.PolicyRevision != ""
		result, err := applications.ReconcileWorkload(ctx, logr.Discard(), cluster.DeepCopy(), cluster, nil)
		require.NoError(t, err)
		require.Equal(t, step.writes, store.writes)
		require.Nil(t, cluster.Status.Workload.LastError)
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster))
		if step.readDenied || step.writeDenied {
			require.Positive(t, result.RequeueAfter)
			if hadVerified {
				require.NotEmpty(t, cluster.Status.Workload.PolicyRevision, "preserve the last complete observation")
			}
			require.Equal(t, "PolicyReconciliationFailed", cluster.Status.Workload.PolicyReconciliation.LastError.Reason)
			if step.deletePolicy {
				require.Error(t, configuration.RequirePolicyReady(cluster, portauth.PolicyNameOperator))
				require.Contains(t, cluster.Status.Workload.PolicyReconciliation.Revisions, portauth.PolicyNameOperator)
				require.Empty(t, cluster.Status.Workload.PolicyReconciliation.Revisions[portauth.PolicyNameOperator])
			} else {
				require.NoError(t, configuration.RequirePolicyReady(cluster, portauth.PolicyNameOperator))
			}
		} else {
			require.Nil(t, cluster.Status.Workload.LastError)
			require.NoError(t, configuration.RequirePolicyReady(cluster, portauth.PolicyNameOperator))
		}
	}
	require.Equal(t, 7, observations, "infrastructure repair still runs when OpenBao policy reads or writes fail")
}

func TestPolicyReconciliationWithoutSelfInit(t *testing.T) {
	cluster := newMinimalClusterObj(newTestNamespace(t), "manual-policy-enrollment")
	waitForOpenBaoClusterAdmissionPolicies(t, cluster.Namespace)
	cluster.Spec.SelfInit = nil
	cluster.Spec.ReconcilePolicies = true
	require.NoError(t, k8sClient.Create(ctx, cluster))
}
