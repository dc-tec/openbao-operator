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

	got := computeScenarioMetrics(before, after)

	assertApproxEqual(t, got[metricReconcileP95], 30)
	assertApproxEqual(t, got[metricBackupLastMax], 240)
	assertApproxEqual(t, got[metricRestoreP95], 1200)
	assertApproxEqual(t, got[metricUpgradeP95], 1800)
	assertApproxEqual(t, got[metricUpgradePodP95], 600)
	assertApproxEqual(t, got[metricWorkqueueRetries], 12)
	assertApproxEqual(t, got[metricReconcileErrRatio], 0.05)
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
	measured := computeScenarioMetrics(before, after)

	passThresholds := scenarioThresholds{
		Metrics: map[string]float64{
			metricReconcileP95:      35,
			metricBackupLastMax:     300,
			metricRestoreP95:        1300,
			metricUpgradeP95:        2000,
			metricUpgradePodP95:     700,
			metricWorkqueueRetries:  20,
			metricReconcileErrRatio: 0.1,
		},
	}
	if findings := compareScenarioMetricsDetailed("fixture-pass", measured, passThresholds).Findings; len(findings) != 0 {
		t.Fatalf("expected pass thresholds, got findings: %v", findings)
	}

	failThresholds := scenarioThresholds{
		Metrics: map[string]float64{
			metricReconcileP95:      20,
			metricBackupLastMax:     300,
			metricRestoreP95:        1300,
			metricUpgradeP95:        2000,
			metricUpgradePodP95:     700,
			metricWorkqueueRetries:  20,
			metricReconcileErrRatio: 0.1,
		},
	}
	findings := compareScenarioMetricsDetailed("fixture-fail", measured, failThresholds).Findings
	if len(findings) == 0 {
		t.Fatalf("expected at least one finding for fail thresholds")
	}
}

func assertApproxEqual(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("value mismatch: got=%v want=%v", got, want)
	}
}
