package configuration

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type policyStore struct {
	values  map[string]string
	deny    string
	readErr error
	reads   []string
	writes  []string
}

func (s *policyStore) ReadACLPolicy(_ context.Context, name string) (*string, error) {
	s.reads = append(s.reads, name)
	if s.readErr != nil {
		return nil, s.readErr
	}
	value, exists := s.values[name]
	if !exists {
		return nil, nil
	}
	return &value, nil
}

func (s *policyStore) WriteACLPolicy(_ context.Context, name, policy string) error {
	s.writes = append(s.writes, name)
	if name == s.deny {
		return portopenbao.NewAPIError("approval denied", http.StatusForbidden, nil)
	}
	s.values[name] = policy
	return nil
}

func TestPolicyReconciliation(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Spec.ReconcilePolicies = true // Manual enrollment does not require self-init.
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
	store := &policyStore{values: map[string]string{}}
	manager := &PolicyManager{ClientFor: func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyClient, error) {
		return store, nil
	}}
	_, err := manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.Empty(t, store.values)
	cluster.Status.Initialized = true
	_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
	require.NoError(t, err)
	require.Len(t, store.writes, 4)
	require.NotContains(t, store.values, portauth.PolicyNameApproval)
	complete := cluster.Status.Workload.PolicyRevision

	for _, tc := range []struct {
		name   string
		change func()
		writes []string
		err    string
	}{
		{name: "unchanged"},
		{name: "deleted", change: func() { delete(store.values, portauth.PolicyNameOperator) }, writes: []string{portauth.PolicyNameOperator}},
		{name: "drift", change: func() { store.values[portauth.PolicyNameRestore] += "\n" }, writes: []string{portauth.PolicyNameRestore}},
		{name: "transient read", change: func() { store.readErr = fmt.Errorf("connection reset") }, err: "connection reset"},
		{name: "read recovery", change: func() { store.readErr = nil }},
		{name: "partial repair", change: func() {
			delete(store.values, portauth.PolicyNameUpgrade)
			delete(store.values, portauth.PolicyNameBackup)
			store.deny = portauth.PolicyNameUpgrade
		}, writes: []string{portauth.PolicyNameUpgrade, portauth.PolicyNameBackup}, err: "approval denied"},
		{name: "write recovery", change: func() { store.deny = "" }, writes: []string{portauth.PolicyNameUpgrade}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster.Status.Workload.PolicyReconciliation.RetryAfter = nil
			if tc.change != nil {
				tc.change()
			}
			store.reads, store.writes = nil, nil
			result, err := manager.Reconcile(t.Context(), logr.Discard(), cluster)
			require.Equal(t, tc.writes, store.writes)
			require.Len(t, store.reads, 4, "one policy failure must not stop the rest")
			require.Equal(t, complete, cluster.Status.Workload.PolicyRevision, "retain last complete observation")
			require.NoError(t, RequirePolicyReady(cluster, portauth.PolicyNameBackup))
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				require.Positive(t, result.RequeueAfter)
				store.reads, store.writes = nil, nil
				_, err = manager.Reconcile(t.Context(), logr.Discard(), cluster)
				require.NoError(t, err, "cooldown is recorded in status")
				require.Empty(t, store.reads)
				if store.deny != "" {
					require.Equal(t, 5*time.Minute, result.RequeueAfter)
					require.Error(t, RequirePolicyReady(cluster, portauth.PolicyNameUpgrade))
				} else {
					require.NoError(t, RequirePolicyReady(cluster, portauth.PolicyNameUpgrade), "read failures retain observations")
				}
				return
			}
			require.NoError(t, err)
			require.NoError(t, RequirePolicyReady(cluster, portauth.PolicyNameUpgrade))
		})
	}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{Strategy: "BlueGreen"}
	require.Error(t, RequirePolicyReady(cluster, portauth.PolicyNameUpgrade))
	require.NoError(t, RequirePolicyReady(cluster, portauth.PolicyNameBackup), "an upgrade permission change leaves backup ready")
	cluster.Spec.ReconcilePolicies = false
	require.NoError(t, RequirePolicyReady(cluster, portauth.PolicyNameUpgrade))
}

func TestPolicyAuthenticationBackoff(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			cluster.Spec.ReconcilePolicies, cluster.Status.Initialized = true, true
			calls := 0
			manager := &PolicyManager{ClientFor: func(context.Context, *openbaov1alpha1.OpenBaoCluster) (portopenbao.PolicyClient, error) {
				calls++
				return nil, portopenbao.NewAPIError("login", code, nil)
			}}
			result, err := manager.Reconcile(t.Context(), logr.Discard(), cluster)
			require.Error(t, err)
			want := 5 * time.Minute
			if code == http.StatusServiceUnavailable {
				want = 30 * time.Second
			}
			require.Equal(t, want, result.RequeueAfter)
			_, _ = manager.Reconcile(t.Context(), logr.Discard(), cluster)
			require.Equal(t, 1, calls)
			cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
			_, _ = manager.Reconcile(t.Context(), logr.Discard(), cluster)
			require.Equal(t, 2, calls, "new desired permissions bypass the old retry deadline")
		})
	}
}
