package observability

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestObserveRestoreStates(t *testing.T) {
	namespace := t.Name()
	request := api.OpenBaoRestore{ObjectMeta: metav1.ObjectMeta{Name: "request", Namespace: namespace}, Spec: api.OpenBaoRestoreSpec{Cluster: "target"}}
	for _, tc := range []struct {
		name   string
		status api.OpenBaoRestoreStatus
		want   float64
	}{
		{"running", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseRunning}, 1},
		{"unknown", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseUnknown}, 4},
		{"restarting", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseUnknown, Restart: &api.RestoreRestartStatus{}}, 5},
		{"resumed", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseUnknown, AdministratorDisposition: api.RestoreAdministratorResume}, 6},
		{"abandoned", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseUnknown, AdministratorDisposition: api.RestoreAdministratorAbandon}, 7},
		{"completed", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseCompleted, AdministratorDisposition: api.RestoreAdministratorResume}, 2},
		{"failed", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed}, 3},
		{"blocked cleanup", api.OpenBaoRestoreStatus{Phase: api.RestorePhaseFailed, Target: &api.RestoreTargetStatus{Cleanup: api.RestoreTargetCleanupFailed}}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request.Status = tc.status
			// No transition event is needed, including after process restart.
			restoreStateGauge.DeleteLabelValues(namespace, "target")
			ObserveRestoreStates(namespace, []api.OpenBaoRestore{request})
			require.Equal(t, tc.want, testutil.ToFloat64(restoreStateGauge.WithLabelValues(namespace, "target")))
		})
	}
	ObserveRestoreStates(namespace, nil)
	require.False(t, restoreStateGauge.DeleteLabelValues(namespace, "target"), "deletion must remove stale series")
}

func TestObserveRestoreStatesPrefersUnreleasedRecovery(t *testing.T) {
	namespace := t.Name()
	older := api.OpenBaoRestore{ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: namespace, CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
		Spec: api.OpenBaoRestoreSpec{Cluster: "target"}, Status: api.OpenBaoRestoreStatus{Phase: api.RestorePhaseUnknown}}
	newer := older.DeepCopy()
	newer.Name, newer.CreationTimestamp = "newer", metav1.NewTime(time.Unix(2, 0))
	newer.Status.Phase = api.RestorePhaseFailed
	for _, requests := range [][]api.OpenBaoRestore{{older, *newer}, {*newer, older}} {
		ObserveRestoreStates(namespace, requests)
		require.Equal(t, float64(4), testutil.ToFloat64(restoreStateGauge.WithLabelValues(namespace, "target")))
	}
	older.Status.AdministratorDisposition = api.RestoreAdministratorResume
	ObserveRestoreStates(namespace, []api.OpenBaoRestore{*newer, older})
	require.Equal(t, float64(3), testutil.ToFloat64(restoreStateGauge.WithLabelValues(namespace, "target")))
	ObserveRestoreStates(namespace, nil)
}
