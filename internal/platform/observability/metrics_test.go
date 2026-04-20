package observability

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

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

func TestClusterMetrics_NoPanic(t *testing.T) {
	m := NewClusterMetrics("ns", "name")

	m.SetReadyReplicas(3)
	m.SetReadReplicaCounts(2, 2, 2, 2)
	m.SetPhase(openbaov1alpha1.ClusterPhaseInitializing)
	m.SetPhase(openbaov1alpha1.ClusterPhaseRunning)
	m.Clear()
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
