package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadScenarioManifest(t *testing.T) {
	t.Parallel()

	manifest, err := loadScenarioManifest("../perf/scenarios.yaml")
	if err != nil {
		t.Fatalf("loadScenarioManifest() error = %v", err)
	}

	byName := scenarioMap(manifest.Scenarios)
	rolling, ok := byName["rolling-upgrade"]
	if !ok {
		t.Fatalf("rolling-upgrade scenario missing")
	}
	if !strings.Contains(rolling.LabelFilter, "!snapshot") {
		t.Fatalf("rolling-upgrade label filter should exclude snapshot coverage: %q", rolling.LabelFilter)
	}
	if _, ok := rolling.MetricPolicies[metricBackupLastMax]; ok {
		t.Fatalf("rolling-upgrade should not evaluate backup duration")
	}
	if rolling.MetricPolicies[metricWorkqueueRetries].Severity != metricSeverityWarn {
		t.Fatalf("workqueue retries should be diagnostic warning signal")
	}
}

func TestSelectedScenariosRejectsUnknownManifestScenario(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "scenarios.yaml")
	if err := os.WriteFile(path, []byte(`
version: v1
scenarios:
  - name: lifecycle
    labelFilter: lifecycle
    metricPolicies:
      reconcile_p95_seconds:
        policy: upper_bound
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := selectedScenarios(options{
		ScenarioPath:  path,
		ScenarioNames: []string{"missing"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown scenario "missing"`) {
		t.Fatalf("expected unknown scenario error, got %v", err)
	}
}
