package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSummarizeValues(t *testing.T) {
	got := summarizeValues([]float64{10, 4, 6})
	if got.Median != 6 {
		t.Fatalf("median = %v, want 6", got.Median)
	}
	if got.UpperSample != 10 {
		t.Fatalf("upper sample = %v, want 10", got.UpperSample)
	}
	if got.Min != 4 || got.Max != 10 || got.Count != 3 {
		t.Fatalf("summary = %+v", got)
	}
}

func TestCompareMeasurementsRequiresAbsoluteAndRelativeRegression(t *testing.T) {
	current := map[string]measurementSummary{
		metricSampleTotalSeconds: {Median: 125, UpperSample: 125, Min: 125, Max: 125, Count: 3},
	}
	baseline := baselineDocument{
		Summary: map[string]measurementSummary{
			metricSampleTotalSeconds: {Median: 100, UpperSample: 100, Min: 100, Max: 100, Count: 7},
		},
	}
	policy := policyDocument{
		Measurements: map[string]measurementPolicy{
			metricSampleTotalSeconds: {
				Policy:          measurementPolicyUpperBound,
				Severity:        measurementSeverityFail,
				Compare:         compareMedian,
				AllowedRelative: 0.20,
				AllowedAbsolute: 30,
				MinimumSamples:  3,
			},
		},
	}

	if findings := compareMeasurements("lifecycle", current, baseline, policy); len(findings) != 0 {
		t.Fatalf("relative-only regression should not fail: %v", findings)
	}

	current[metricSampleTotalSeconds] = measurementSummary{Median: 140, UpperSample: 140, Min: 140, Max: 140, Count: 3}
	findings := compareMeasurements("lifecycle", current, baseline, policy)
	if len(findings) != 1 {
		t.Fatalf("findings len = %d, want 1 (%v)", len(findings), findings)
	}
	if findings[0].Severity != measurementSeverityFail {
		t.Fatalf("severity = %q, want %q", findings[0].Severity, measurementSeverityFail)
	}
}

func TestReportSummarizesSyntheticSamples(t *testing.T) {
	tmp := t.TempDir()
	artifactDir := filepath.Join(tmp, "dist", "perf")
	baselineDir := filepath.Join(tmp, "baselines")
	policyPath := filepath.Join(tmp, "weekly.yaml")

	writeTestFile(t, policyPath, `
version: v2
measurements:
  sample_total_seconds:
    policy: upper_bound
    severity: warn
    compare: median
    allowedRelativeRegression: 0.5
    allowedAbsoluteRegressionSeconds: 10
    minimumSamples: 1
`)
	writeTestJSON(t, filepath.Join(baselineDir, "lifecycle-convergence", "kind-v1.34.3.json"), baselineDocument{
		Version:    versionV2,
		Scenario:   "lifecycle-convergence",
		CapturedAt: time.Unix(0, 0).UTC(),
		Samples: map[string][]float64{
			metricSampleTotalSeconds: {100, 102, 104},
		},
		Summary: map[string]measurementSummary{
			metricSampleTotalSeconds: {Median: 102, UpperSample: 104, Min: 100, Max: 104, Count: 3},
		},
	})
	writeTestJSON(t, filepath.Join(artifactDir, "scenarios", "lifecycle-convergence", "sample-001.json"), sampleDocument{
		Version:  versionV2,
		Scenario: "lifecycle-convergence",
		Sample:   1,
		Status:   sampleStatusPass,
		Measurements: map[string]float64{
			metricSampleTotalSeconds: 105,
		},
	})

	opts := defaultOptions("report")
	opts.ArtifactDir = artifactDir
	opts.BaselineDir = baselineDir
	opts.PolicyPath = policyPath
	opts.ReportOut = filepath.Join(tmp, "report.md")
	opts.SummaryOut = filepath.Join(tmp, "summary.json")

	if err := runReport(opts); err != nil {
		t.Fatalf("runReport() error = %v", err)
	}
	report, err := os.ReadFile(opts.ReportOut)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(report), "lifecycle-convergence") {
		t.Fatalf("report should mention scenario, got:\n%s", string(report))
	}
	if !strings.Contains(string(report), metricSampleTotalSeconds) {
		t.Fatalf("report should mention measurement, got:\n%s", string(report))
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := writeJSONFile(path, value); err != nil {
		t.Fatalf("write json %s: %v", path, err)
	}
}
