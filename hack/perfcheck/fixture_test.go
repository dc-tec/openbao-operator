package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeScenarioMetricsFromFixtures(t *testing.T) {
	beforeText, err := os.ReadFile(filepath.Clean("testdata/metrics_before.prom"))
	if err != nil {
		t.Fatalf("read before fixture: %v", err)
	}
	afterText, err := os.ReadFile(filepath.Clean("testdata/metrics_after.prom"))
	if err != nil {
		t.Fatalf("read after fixture: %v", err)
	}

	before, err := parseMetricsSnapshot(string(beforeText))
	if err != nil {
		t.Fatalf("parse before fixture: %v", err)
	}
	after, err := parseMetricsSnapshot(string(afterText))
	if err != nil {
		t.Fatalf("parse after fixture: %v", err)
	}

	got := computeDiagnosticMeasurements(before, after)

	assertApproxEqual(t, got[metricReconcileDurationBucketP95], 30)
	assertApproxEqual(t, got[metricBackupLastDurationSeconds], 240)
	assertApproxEqual(t, got[metricRestoreDurationBucketP95], 1200)
	assertApproxEqual(t, got[metricUpgradeDurationBucketP95], 1800)
	assertApproxEqual(t, got[metricUpgradePodDurationBucketP95], 600)
	assertApproxEqual(t, got[metricWorkqueueRetriesDelta], 12)
	assertApproxEqual(t, got[metricReconcileErrorRatio], 0.05)
}

func TestFixtureDrivenThresholdPassAndFail(t *testing.T) {
	beforeText, err := os.ReadFile(filepath.Clean("testdata/metrics_before.prom"))
	if err != nil {
		t.Fatalf("read before fixture: %v", err)
	}
	afterText, err := os.ReadFile(filepath.Clean("testdata/metrics_after.prom"))
	if err != nil {
		t.Fatalf("read after fixture: %v", err)
	}

	before, err := parseMetricsSnapshot(string(beforeText))
	if err != nil {
		t.Fatalf("parse before fixture: %v", err)
	}
	after, err := parseMetricsSnapshot(string(afterText))
	if err != nil {
		t.Fatalf("parse after fixture: %v", err)
	}
	measured := computeDiagnosticMeasurements(before, after)

	passBaseline := baselineDocument{
		Summary: map[string]measurementSummary{
			metricReconcileDurationBucketP95:  {Median: 35, UpperSample: 35, Min: 35, Max: 35, Count: 3},
			metricBackupLastDurationSeconds:   {Median: 300, UpperSample: 300, Min: 300, Max: 300, Count: 3},
			metricRestoreDurationBucketP95:    {Median: 1300, UpperSample: 1300, Min: 1300, Max: 1300, Count: 3},
			metricUpgradeDurationBucketP95:    {Median: 2000, UpperSample: 2000, Min: 2000, Max: 2000, Count: 3},
			metricUpgradePodDurationBucketP95: {Median: 700, UpperSample: 700, Min: 700, Max: 700, Count: 3},
			metricWorkqueueRetriesDelta:       {Median: 20, UpperSample: 20, Min: 20, Max: 20, Count: 3},
			metricReconcileErrorRatio:         {Median: 0.1, UpperSample: 0.1, Min: 0.1, Max: 0.1, Count: 3},
		},
	}
	policy := policyDocument{
		Defaults: measurementPolicy{
			Policy:   measurementPolicyUpperBound,
			Severity: measurementSeverityFail,
			Compare:  compareUpperSample,
		},
		Measurements: map[string]measurementPolicy{
			metricReconcileDurationBucketP95:  {},
			metricBackupLastDurationSeconds:   {},
			metricRestoreDurationBucketP95:    {},
			metricUpgradeDurationBucketP95:    {},
			metricUpgradePodDurationBucketP95: {},
			metricWorkqueueRetriesDelta:       {},
			metricReconcileErrorRatio:         {},
		},
	}
	current := make(map[string]measurementSummary, len(passBaseline.Summary))
	for key := range passBaseline.Summary {
		value := measured[key]
		current[key] = measurementSummary{Median: value, UpperSample: value, Min: value, Max: value, Count: 1}
	}
	if findings := compareMeasurements("fixture-pass", current, passBaseline, policy); len(findings) != 0 {
		t.Fatalf("expected baseline comparison to pass, got findings: %v", findings)
	}

	failBaseline := baselineDocument{
		Summary: map[string]measurementSummary{
			metricReconcileDurationBucketP95: {Median: 20, UpperSample: 20, Min: 20, Max: 20, Count: 3},
		},
	}
	findings := compareMeasurements("fixture-fail", current, failBaseline, policyDocument{
		Defaults: measurementPolicy{
			Policy:   measurementPolicyUpperBound,
			Severity: measurementSeverityFail,
			Compare:  compareUpperSample,
		},
		Measurements: map[string]measurementPolicy{
			metricReconcileDurationBucketP95: {},
		},
	})
	if len(findings) == 0 {
		t.Fatalf("expected at least one finding for failing baseline comparison")
	}
}

func assertApproxEqual(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("value mismatch: got=%v want=%v", got, want)
	}
}
