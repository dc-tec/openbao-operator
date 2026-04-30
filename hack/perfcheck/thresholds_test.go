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
				MetricPolicies: map[string]metricPolicySpec{
					metricReconcileP95: {
						Policy:     metricPolicyUpperBound,
						Multiplier: metricMultiplierP95,
					},
					metricBackupLastMax: {
						Policy:     metricPolicyUpperBound,
						Multiplier: metricMultiplierMax,
					},
					metricWorkqueueRetries: {
						Policy:     metricPolicyUpperBound,
						Severity:   metricSeverityWarn,
						Multiplier: metricMultiplierMax,
						Floor:      ptrFloat64(20),
					},
				},
				MaxMetrics: map[string]float64{
					metricReconcileP95:     8,
					metricBackupLastMax:    100,
					metricWorkqueueRetries: 7,
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
	if got := lifecycle.Metrics[metricWorkqueueRetries]; got != 20 {
		t.Fatalf("workqueue threshold = %v, want 20", got)
	}
	if got := lifecycle.MetricPolicies[metricWorkqueueRetries].Severity; got != metricSeverityWarn {
		t.Fatalf("workqueue severity = %q, want %q", got, metricSeverityWarn)
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

	findings := compareScenarioMetricsDetailed("lifecycle", measured, thresholds).Findings
	if len(findings) != 1 {
		t.Fatalf("findings len = %d, want 1 (%v)", len(findings), findings)
	}
	if !strings.Contains(findings[0], metricBackupLastMax) {
		t.Fatalf("finding does not reference exceeded metric: %q", findings[0])
	}
}

func TestCompareScenarioMetricsWarningSeverityDoesNotFail(t *testing.T) {
	thresholds := scenarioThresholds{
		MetricPolicies: map[string]metricPolicySpec{
			metricWorkqueueRetries: {
				Policy:   metricPolicyUpperBound,
				Severity: metricSeverityWarn,
			},
		},
		Metrics: map[string]float64{
			metricWorkqueueRetries: 100,
		},
	}
	measured := map[string]float64{
		metricWorkqueueRetries: 120,
	}

	result := compareScenarioMetricsDetailed("rolling-upgrade", measured, thresholds)
	if len(result.Findings) != 0 {
		t.Fatalf("expected warning-only comparison, got findings: %v", result.Findings)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1 (%v)", len(result.Warnings), result.Warnings)
	}
}

func TestApplyScenarioPolicyFiltersStaleThresholdMetrics(t *testing.T) {
	scenario := scenarioSpec{
		Name:        "rolling-upgrade",
		LabelFilter: "upgrade && rolling && !snapshot",
		MetricPolicies: map[string]metricPolicySpec{
			metricUpgradeP95: {
				Policy: metricPolicyUpperBound,
			},
		},
	}
	thresholds := scenarioThresholds{
		LabelFilter: "upgrade && rolling",
		Metrics: map[string]float64{
			metricUpgradeP95:    75,
			metricBackupLastMax: 0,
		},
	}

	filtered := applyScenarioPolicy(thresholds, scenario)
	if filtered.LabelFilter != scenario.LabelFilter {
		t.Fatalf("labelFilter = %q, want %q", filtered.LabelFilter, scenario.LabelFilter)
	}
	if _, ok := filtered.Metrics[metricBackupLastMax]; ok {
		t.Fatalf("stale backup metric should be filtered from rolling-upgrade thresholds")
	}
	if got := filtered.Metrics[metricUpgradeP95]; got != 75 {
		t.Fatalf("upgrade threshold = %v, want 75", got)
	}
}

func TestValidateScenarioThresholdsRequiresManagedMetrics(t *testing.T) {
	scenario := scenarioSpec{
		Name: "backup-restore",
		MetricPolicies: map[string]metricPolicySpec{
			metricRestoreP95: {
				Policy: metricPolicyUpperBound,
			},
		},
	}

	err := validateScenarioThresholds(scenarioThresholds{Metrics: map[string]float64{}}, scenario)
	if err == nil || !strings.Contains(err.Error(), metricRestoreP95) {
		t.Fatalf("expected missing metric threshold error, got %v", err)
	}
}

func TestBuildThresholdsMustBeZeroPolicy(t *testing.T) {
	baseline := baselineDocument{
		NodeImage:    "kindest/node:v1.34.3",
		CapturedAt:   time.Unix(0, 0).UTC(),
		RunsPerCase:  5,
		Multipliers:  multiplierConfig{P95: 1.25, Max: 1.40},
		MetricSchema: []string{metricBackupLastMax},
		Scenarios: map[string]scenarioBaseline{
			"rolling-upgrade": {
				LabelFilter: "upgrade && rolling",
				MetricPolicies: map[string]metricPolicySpec{
					metricBackupLastMax: {
						Policy: metricPolicyMustBeZero,
					},
				},
				MaxMetrics: map[string]float64{
					metricBackupLastMax: 3,
				},
			},
		},
	}

	thresholds := buildThresholds(baseline)
	if got := thresholds.Scenarios["rolling-upgrade"].Metrics[metricBackupLastMax]; got != 0 {
		t.Fatalf("must_be_zero threshold = %v, want 0", got)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
