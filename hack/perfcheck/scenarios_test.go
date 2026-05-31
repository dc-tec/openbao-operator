package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadScenarioManifest(t *testing.T) {
	t.Parallel()

	manifest, err := loadScenarioManifest("../perf/v2/scenarios.yaml")
	if err != nil {
		t.Fatalf("loadScenarioManifest() error = %v", err)
	}

	byName := scenarioMap(manifest.Scenarios)
	lifecycle, ok := byName["lifecycle-convergence"]
	if !ok {
		t.Fatalf("lifecycle-convergence scenario missing")
	}
	if lifecycle.Executor != executorNativeGo {
		t.Fatalf("lifecycle executor = %q, want %q", lifecycle.Executor, executorNativeGo)
	}
	if lifecycle.LabelFilter != "" {
		t.Fatalf("native lifecycle should not require a Ginkgo label filter: %q", lifecycle.LabelFilter)
	}
	if !containsString(lifecycle.Primary, metricClusterAvailableSeconds) {
		t.Fatalf("lifecycle should include %s as phase-2 primary measurement", metricClusterAvailableSeconds)
	}

	tenantChurn, ok := byName["tenant-churn"]
	if !ok {
		t.Fatalf("tenant-churn scenario missing")
	}
	if tenantChurn.Executor != executorNativeGo {
		t.Fatalf("tenant-churn executor = %q, want %q", tenantChurn.Executor, executorNativeGo)
	}
	if !containsString(tenantChurn.Primary, metricTenantChurnCompleteSeconds) {
		t.Fatalf("tenant-churn should include %s as primary measurement", metricTenantChurnCompleteSeconds)
	}

	backup, ok := byName["backup"]
	if !ok {
		t.Fatalf("backup scenario missing")
	}
	if backup.Executor != executorNativeGo {
		t.Fatalf("backup executor = %q, want %q", backup.Executor, executorNativeGo)
	}
	if backup.LabelFilter != "" {
		t.Fatalf("native backup should not require a Ginkgo label filter: %q", backup.LabelFilter)
	}
	if !containsString(backup.Primary, metricBackupTotalSeconds) {
		t.Fatalf("backup should include %s as primary measurement", metricBackupTotalSeconds)
	}

	restore, ok := byName["restore"]
	if !ok {
		t.Fatalf("restore scenario missing")
	}
	if restore.Executor != executorNativeGo {
		t.Fatalf("restore executor = %q, want %q", restore.Executor, executorNativeGo)
	}
	if restore.LabelFilter != "" {
		t.Fatalf("native restore should not require a Ginkgo label filter: %q", restore.LabelFilter)
	}
	if !containsString(restore.Primary, metricRestoreTotalSeconds) {
		t.Fatalf("restore should include %s as primary measurement", metricRestoreTotalSeconds)
	}
}

func TestSelectedScenariosRejectsUnknownManifestScenario(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "scenarios.yaml")
	if err := os.WriteFile(path, []byte(`
version: v2
scenarios:
  - name: lifecycle
    executor: e2e-ginkgo
    labelFilter: lifecycle
    primaryMeasurements:
      - sample_total_seconds
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, _, err := selectedScenarios(options{
		ScenarioPath:  path,
		ScenarioNames: []string{"missing"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown scenario "missing"`) {
		t.Fatalf("expected unknown scenario error, got %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
