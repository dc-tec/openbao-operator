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

func TestReconcileErrorRatioWhenDenominatorZero(t *testing.T) {
	if got := reconcileErrorRatio(0, 0); got != 0 {
		t.Fatalf("reconcileErrorRatio(0,0) = %v, want 0", got)
	}
	if got := reconcileErrorRatio(2, 0); got != 1 {
		t.Fatalf("reconcileErrorRatio(2,0) = %v, want 1", got)
	}
}
