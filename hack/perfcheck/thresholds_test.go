package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildThresholdsFromBaseline(t *testing.T) {
	baseline := baselineDocument{
		NodeImage:   "kindest/node:v1.34.3",
		CapturedAt:  time.Unix(0, 0).UTC(),
		RunsPerCase: 5,
		Multipliers: multiplierConfig{P95: 1.25, Max: 1.40},
		MetricSchema: []string{
			metricReconcileP95,
			metricBackupLastMax,
		},
		Scenarios: map[string]scenarioBaseline{
			"lifecycle": {
				LabelFilter: "lifecycle && critical",
				MaxMetrics: map[string]float64{
					metricReconcileP95:  8,
					metricBackupLastMax: 100,
				},
			},
		},
	}

	thresholds := buildThresholds(baseline)
	lifecycle := thresholds.Scenarios["lifecycle"]

	if got := lifecycle.Metrics[metricReconcileP95]; got != 10 {
		t.Fatalf("reconcile threshold = %v, want 10", got)
	}
	if got := lifecycle.Metrics[metricBackupLastMax]; got != 140 {
		t.Fatalf("backup threshold = %v, want 140", got)
	}
}

func TestCompareScenarioMetricsFindings(t *testing.T) {
	thresholds := scenarioThresholds{
		Metrics: map[string]float64{
			metricBackupLastMax: 100,
			metricReconcileP95:  10,
		},
	}
	measured := map[string]float64{
		metricBackupLastMax: 120,
		metricReconcileP95:  9,
	}

	findings := compareScenarioMetrics("lifecycle", measured, thresholds)
	if len(findings) != 1 {
		t.Fatalf("findings len = %d, want 1 (%v)", len(findings), findings)
	}
	if !strings.Contains(findings[0], metricBackupLastMax) {
		t.Fatalf("finding does not reference exceeded metric: %q", findings[0])
	}
}
