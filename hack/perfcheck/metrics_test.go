package main

import (
	"math"
	"testing"
)

func TestHistogramQuantileUpperBound(t *testing.T) {
	buckets := map[float64]float64{
		0.5:         4,
		1:           8,
		2:           12,
		5:           18,
		10:          20,
		math.Inf(1): 20,
	}

	got := histogramP95UpperBound(buckets)
	if got != 10 {
		t.Fatalf("histogramP95UpperBound() = %v, want 10", got)
	}
}

func TestCounterDeltaClampsResets(t *testing.T) {
	before := metricsSnapshot{Counters: map[string]float64{"x": 20}}
	after := metricsSnapshot{Counters: map[string]float64{"x": 10}}

	got := counterDelta(before, after, "x")
	if got != 0 {
		t.Fatalf("counterDelta() = %v, want 0", got)
	}
}

func TestComputeDiagnosticMeasurementsOmitsMissingMetrics(t *testing.T) {
	before := emptySnapshot()
	after := emptySnapshot()

	got := computeDiagnosticMeasurements(before, after)
	if len(got) != 0 {
		t.Fatalf("diagnostics for empty snapshots = %+v, want empty", got)
	}

	before.Counters["openbao_client_requests_total"] = 7
	after.Counters["openbao_client_requests_total"] = 7
	got = computeDiagnosticMeasurements(before, after)
	if _, exists := got[metricOpenBaoAPIRequests]; !exists {
		t.Fatalf("%s should be present when source counter exists", metricOpenBaoAPIRequests)
	}
	if _, exists := got[metricOpenBaoAuthLogins]; exists {
		t.Fatalf("%s should be omitted when source counter is absent", metricOpenBaoAuthLogins)
	}
}

func TestReconcileErrorRatioWhenDenominatorZero(t *testing.T) {
	if got := reconcileErrorRatio(0, 0); got != 0 {
		t.Fatalf("reconcileErrorRatio(0,0) = %v, want 0", got)
	}
	if got := reconcileErrorRatio(2, 0); got != 1 {
		t.Fatalf("reconcileErrorRatio(2,0) = %v, want 1", got)
	}
}
