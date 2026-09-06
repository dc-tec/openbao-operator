package observability

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestReconcileMetrics_NoPanic(t *testing.T) {
	m := NewReconcileMetrics("ns", "name", "ctrl")

	// These calls should not panic and will register/update metrics for the
	// given label set.
	m.ObserveDuration(0.5)
	m.ObserveDuration(1.0)
	m.IncrementError("Error")
}

func TestClusterMetrics_SetPhase(t *testing.T) {
	tests := []struct {
		name   string
		phases []openbaov1alpha1.ClusterPhase
	}{
		{
			name: "phase transitions",
			phases: []openbaov1alpha1.ClusterPhase{
				openbaov1alpha1.ClusterPhaseInitializing,
				openbaov1alpha1.ClusterPhaseRunning,
				openbaov1alpha1.ClusterPhaseUpgrading,
				openbaov1alpha1.ClusterPhaseBackingUp,
				openbaov1alpha1.ClusterPhaseFailed,
				openbaov1alpha1.ClusterPhaseRunning,
			},
		},
		{
			name: "repeated phase",
			phases: []openbaov1alpha1.ClusterPhase{
				openbaov1alpha1.ClusterPhaseRunning,
				openbaov1alpha1.ClusterPhaseRunning,
			},
		},
		{
			name: "empty phase",
			phases: []openbaov1alpha1.ClusterPhase{
				"",
				openbaov1alpha1.ClusterPhaseRunning,
				"",
				openbaov1alpha1.ClusterPhaseInitializing,
			},
		},
		{
			name: "unrecognized phase",
			phases: []openbaov1alpha1.ClusterPhase{
				openbaov1alpha1.ClusterPhaseRunning,
				"Unrecognized",
				openbaov1alpha1.ClusterPhaseInitializing,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, name := "ns", t.Name()
			t.Cleanup(NewClusterMetrics(namespace, name).Clear)
			for _, phase := range tt.phases {
				// Reconciliation creates a new helper for each update.
				NewClusterMetrics(namespace, name).SetPhase(phase)
				want := map[openbaov1alpha1.ClusterPhase]float64{
					openbaov1alpha1.ClusterPhaseInitializing: 0,
					openbaov1alpha1.ClusterPhaseRunning:      0,
					openbaov1alpha1.ClusterPhaseUpgrading:    0,
					openbaov1alpha1.ClusterPhaseBackingUp:    0,
					openbaov1alpha1.ClusterPhaseFailed:       0,
				}
				if _, known := want[phase]; known {
					want[phase] = 1
				}
				got := gatherClusterPhases(t, namespace, name)
				for knownPhase, value := range want {
					require.Equal(t, value, got[knownPhase], "active phase %q, exported phase %q", phase, knownPhase)
				}
				for exportedPhase := range got {
					require.Contains(t, want, exportedPhase, "only known phase labels are exported")
				}
			}
		})
	}
}

func TestClusterMetrics_PhaseIsolationAndClear(t *testing.T) {
	for _, tt := range []struct {
		name      string
		namespace string
		cluster   string
	}{
		{name: "same namespace", namespace: "ns", cluster: "other"},
		{name: "same name", namespace: "other-ns", cluster: "cluster"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewClusterMetrics("ns", "cluster")
			other := NewClusterMetrics(tt.namespace, tt.cluster)
			t.Cleanup(m.Clear)
			t.Cleanup(other.Clear)
			other.SetPhase(openbaov1alpha1.ClusterPhaseBackingUp)
			otherBefore := gatherClusterPhases(t, tt.namespace, tt.cluster)

			m.SetPhase(openbaov1alpha1.ClusterPhaseInitializing)
			m.SetPhase(openbaov1alpha1.ClusterPhaseRunning)
			require.Equal(t, otherBefore, gatherClusterPhases(t, tt.namespace, tt.cluster))

			m.Clear()
			m.Clear()
			require.Empty(t, gatherClusterPhases(t, "ns", "cluster"))
			require.Equal(t, otherBefore, gatherClusterPhases(t, tt.namespace, tt.cluster))
		})
	}
}

func gatherClusterPhases(t *testing.T, namespace, name string) map[openbaov1alpha1.ClusterPhase]float64 {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(clusterPhaseGauge)
	families, err := registry.Gather()
	require.NoError(t, err)
	phases := make(map[openbaov1alpha1.ClusterPhase]float64)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string)
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["namespace"] == namespace && labels["name"] == name {
				phases[openbaov1alpha1.ClusterPhase(labels["phase"])] = metric.GetGauge().GetValue()
			}
		}
	}
	return phases
}

func TestReconcileMetrics_EmitsSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())
	controller := fmt.Sprintf("ctrl-%s", t.Name())

	durationBefore := testutil.CollectAndCount(reconcileDurationHistogram)
	errorsBefore := testutil.CollectAndCount(reconcileErrorsTotal)

	m := NewReconcileMetrics(namespace, name, controller)
	m.ObserveDuration(0.1)
	m.IncrementError("TestError")

	durationAfter := testutil.CollectAndCount(reconcileDurationHistogram)
	errorsAfter := testutil.CollectAndCount(reconcileErrorsTotal)

	if durationAfter != durationBefore+1 {
		t.Fatalf("expected reconcile duration series to increase by 1 (before=%d, after=%d)", durationBefore, durationAfter)
	}
	if errorsAfter != errorsBefore+1 {
		t.Fatalf("expected reconcile error series to increase by 1 (before=%d, after=%d)", errorsBefore, errorsAfter)
	}
}

func TestRestoreMetrics_EmitsSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())

	totalBefore := testutil.CollectAndCount(restoreTotal)
	successBefore := testutil.CollectAndCount(restoreSuccessTotal)
	failureBefore := testutil.CollectAndCount(restoreFailureTotal)
	durationBefore := testutil.CollectAndCount(restoreDurationHistogram)

	m := NewRestoreMetrics(namespace, name)
	m.RecordStarted()
	m.RecordSuccess(1.0)
	m.RecordFailureWithDuration(2.0)

	totalAfter := testutil.CollectAndCount(restoreTotal)
	successAfter := testutil.CollectAndCount(restoreSuccessTotal)
	failureAfter := testutil.CollectAndCount(restoreFailureTotal)
	durationAfter := testutil.CollectAndCount(restoreDurationHistogram)

	if totalAfter != totalBefore+1 {
		t.Fatalf("expected restore_total series to increase by 1 (before=%d, after=%d)", totalBefore, totalAfter)
	}
	if successAfter != successBefore+1 {
		t.Fatalf("expected restore_success_total series to increase by 1 (before=%d, after=%d)", successBefore, successAfter)
	}
	if failureAfter != failureBefore+1 {
		t.Fatalf("expected restore_failure_total series to increase by 1 (before=%d, after=%d)", failureBefore, failureAfter)
	}
	// RecordSuccess + RecordFailureWithDuration should create exactly one histogram series.
	if durationAfter != durationBefore+1 {
		t.Fatalf("expected restore duration histogram series to increase by 1 (before=%d, after=%d)", durationBefore, durationAfter)
	}
}

func TestClusterMetrics_ReadReplicaSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())

	desiredBefore := testutil.CollectAndCount(clusterReadReplicasDesiredGauge)
	readyBefore := testutil.CollectAndCount(clusterReadReplicasReadyGauge)
	registeredBefore := testutil.CollectAndCount(clusterReadReplicasRegisteredGauge)
	healthyBefore := testutil.CollectAndCount(clusterReadReplicasHealthyGauge)

	m := NewClusterMetrics(namespace, name)
	m.SetReadReplicaCounts(2, 2, 2, 1)

	desiredAfter := testutil.CollectAndCount(clusterReadReplicasDesiredGauge)
	readyAfter := testutil.CollectAndCount(clusterReadReplicasReadyGauge)
	registeredAfter := testutil.CollectAndCount(clusterReadReplicasRegisteredGauge)
	healthyAfter := testutil.CollectAndCount(clusterReadReplicasHealthyGauge)

	if desiredAfter != desiredBefore+1 {
		t.Fatalf("expected read replica desired series to increase by 1 (before=%d, after=%d)", desiredBefore, desiredAfter)
	}
	if readyAfter != readyBefore+1 {
		t.Fatalf("expected read replica ready series to increase by 1 (before=%d, after=%d)", readyBefore, readyAfter)
	}
	if registeredAfter != registeredBefore+1 {
		t.Fatalf("expected read replica registered series to increase by 1 (before=%d, after=%d)", registeredBefore, registeredAfter)
	}
	if healthyAfter != healthyBefore+1 {
		t.Fatalf("expected read replica healthy series to increase by 1 (before=%d, after=%d)", healthyBefore, healthyAfter)
	}

	m.Clear()
}
