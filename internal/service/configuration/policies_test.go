package configuration

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type policyStore struct {
	values map[string]string
	deny   string
}

func (s *policyStore) WriteACLPolicy(_ context.Context, name, policy string) error {
	if name == s.deny {
		return fmt.Errorf("approval denied")
	}
	s.values[name] = policy
	return nil
}

func TestPolicyReconciliation(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true, ReconcilePolicies: true}}
	store := &policyStore{values: map[string]string{}}
	manager := &PolicyManager{ClientFor: func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyWriter, error) {
		return store, nil
	}}
	_, err := manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.Empty(t, store.values, "uninitialized clusters do not authenticate")
	cluster.Status.Initialized = true
	_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NoError(t, RequirePoliciesReady(cluster))
	require.Len(t, store.values, 3)
	require.NotContains(t, store.values, portauth.PolicyNameApproval)

	delete(store.values, portauth.PolicyNameOperator)
	_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NotEmpty(t, store.values[portauth.PolicyNameOperator], "external deletion is repaired")

	store.deny = portauth.PolicyNameUpgrade
	_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.ErrorContains(t, err, "approval denied")
	require.Empty(t, cluster.Status.Workload.PolicyRevision, "partial writes cannot publish readiness")
	require.Error(t, RequirePoliciesReady(cluster))
	store.deny = ""
	_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NoError(t, RequirePoliciesReady(cluster))
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
	require.Error(t, RequirePoliciesReady(cluster), "configuration changes invalidate the previous bundle")
	cluster.Spec.SelfInit.OIDC.ReconcilePolicies = false
	require.NoError(t, RequirePoliciesReady(cluster))
	_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.NotContains(t, store.values, portauth.PolicyNameBackup, "disabled reconciliation makes no writes")
}
