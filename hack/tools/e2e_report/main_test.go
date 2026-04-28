package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2/types"
)

func writeTempGinkgoReport(t *testing.T, reports []types.Report) string {
	t.Helper()

	data, err := json.Marshal(reports)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	path := filepath.Join(t.TempDir(), "ginkgo.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

func TestLoadReports(t *testing.T) {
	reportPath := writeTempGinkgoReport(t, []types.Report{
		{
			SuiteSucceeded: true,
			PreRunStats: types.PreRunStats{
				TotalSpecs:       2,
				SpecsThatWillRun: 1,
			},
		},
	})

	reports, err := loadReports([]string{reportPath})
	if err != nil {
		t.Fatalf("loadReports() error = %v", err)
	}
	if got := len(reports); got != 1 {
		t.Fatalf("len(reports) = %d, want 1", got)
	}
	if reports[0].PreRunStats.TotalSpecs != 2 {
		t.Fatalf("total specs = %d, want 2", reports[0].PreRunStats.TotalSpecs)
	}
}

func TestLoadReportsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ginkgo.json")
	if err := os.WriteFile(path, []byte(`{not-json}`), 0o644); err != nil {
		t.Fatalf("write malformed report: %v", err)
	}

	_, err := loadReports([]string{path})
	if err == nil {
		t.Fatalf("loadReports() error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse report") {
		t.Fatalf("error = %q, want parse report message", err)
	}
}

func TestBuildSummaryCountsAndSlowestSpecs(t *testing.T) {
	reports := []types.Report{
		{
			SuiteSucceeded: true,
			PreRunStats: types.PreRunStats{
				TotalSpecs:       4,
				SpecsThatWillRun: 3,
			},
			RunTime: 5 * time.Second,
			SpecReports: types.SpecReports{
				{
					ContainerHierarchyTexts:  []string{"lifecycle"},
					ContainerHierarchyLabels: [][]string{{"lifecycle"}},
					LeafNodeText:             "creates a cluster",
					LeafNodeLabels:           []string{"case:lifecycle-create"},
					State:                    types.SpecStatePassed,
					RunTime:                  2 * time.Second,
				},
				{
					ContainerHierarchyTexts: []string{"backup"},
					LeafNodeText:            "restores a snapshot",
					State:                   types.SpecStateFailed,
					RunTime:                 3 * time.Second,
					Failure: types.Failure{
						Message: "restore failed\nwith details",
					},
				},
				{
					ContainerHierarchyTexts: []string{"openshift"},
					LeafNodeText:            "runs on openshift",
					State:                   types.SpecStateSkipped,
					RunTime:                 0,
				},
			},
		},
	}

	s := buildSummary(reports, options{
		TopSpecs:          2,
		Lane:              "Backup & Restore",
		Selector:          "restore",
		KubernetesVersion: "kindest/node:v1.35.1",
		OpenBAOVersion:    "ghcr.io/openbao/openbao:2.5.3",
	})

	if !s.SuiteSucceeded {
		t.Fatalf("suite succeeded = false, want true")
	}
	if s.Passed != 1 || s.Failed != 1 || s.Skipped != 1 {
		t.Fatalf("counts = passed %d failed %d skipped %d, want 1/1/1", s.Passed, s.Failed, s.Skipped)
	}
	if got := len(s.SlowestSpecs); got != 2 {
		t.Fatalf("slowest specs = %d, want 2", got)
	}
	if s.SlowestSpecs[0].Name != "backup restores a snapshot" {
		t.Fatalf("slowest[0] = %q, want backup spec", s.SlowestSpecs[0].Name)
	}
	if got := s.Failures[0].FailureMessage; got != "restore failed" {
		t.Fatalf("failure message = %q, want first line", got)
	}
}

func TestFormatMarkdown(t *testing.T) {
	out := formatMarkdown(summary{
		Lane:             "Core",
		SuiteSucceeded:   true,
		TotalSpecs:       2,
		SpecsThatWillRun: 2,
		Passed:           2,
		RunTime:          "1s",
		SlowestSpecs: []specSummary{
			{Name: "lifecycle creates | updates", State: "passed", RunTime: "1s"},
		},
	})

	if !strings.Contains(out, "## E2E Report Summary") {
		t.Fatalf("markdown missing title: %q", out)
	}
	if !strings.Contains(out, "lifecycle creates \\| updates") {
		t.Fatalf("markdown did not escape table pipe: %q", out)
	}
}
