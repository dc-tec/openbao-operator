package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempReport(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "mutation-report.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp report: %v", err)
	}
	return path
}

func TestLoadReports(t *testing.T) {
	reportPath := writeTempReport(t, `{
  "totalFiles": 1,
  "processedFiles": 1,
  "totalMutants": 2,
  "killedMutants": 1,
  "results": [
    {
      "mutant": {
        "filePath": "internal/example/example.go",
        "line": 10,
        "column": 4,
        "type": "conditional_binary",
        "description": "Replace == with !="
      },
      "status": "KILLED"
    },
    {
      "mutant": {
        "filePath": "internal/example/example.go",
        "line": 11,
        "column": 4,
        "type": "logical_binary",
        "description": "Replace && with ||"
      },
      "status": "SURVIVED"
    }
  ],
  "statistics": {
    "killed": 1,
    "survived": 1,
    "timedOut": 0,
    "errors": 0,
    "notViable": 0,
    "mutationScore": 50
  }
}`)

	reports, err := loadReports([]string{reportPath})
	if err != nil {
		t.Fatalf("loadReports() error = %v", err)
	}
	if got := len(reports); got != 1 {
		t.Fatalf("len(reports) = %d, want 1", got)
	}
	if reports[0].source != reportPath {
		t.Fatalf("report source = %q, want %q", reports[0].source, reportPath)
	}
	if reports[0].TotalMutants != 2 {
		t.Fatalf("total mutants = %d, want 2", reports[0].TotalMutants)
	}
}

func TestLoadReportsMalformedJSON(t *testing.T) {
	reportPath := writeTempReport(t, `{not-json}`)

	_, err := loadReports([]string{reportPath})
	if err == nil {
		t.Fatalf("loadReports() error = nil, want parse failure")
	}
	if !strings.Contains(err.Error(), "parse report") {
		t.Fatalf("error = %q, want parse report message", err)
	}
}

func TestBuildSummaryZeroMutants(t *testing.T) {
	reports := []mutationReport{
		{
			TotalFiles:     2,
			ProcessedFiles: 2,
			TotalMutants:   0,
			KilledMutants:  0,
			Results:        []mutationResult{},
			Statistics: reportStatistics{
				Killed:    0,
				Survived:  0,
				TimedOut:  0,
				Errors:    0,
				NotViable: 0,
				Score:     0,
			},
			source: "zero-mutants.json",
		},
	}

	s := buildSummary(reports, 10)
	if s.Aggregated.TotalMutants != 0 {
		t.Fatalf("total mutants = %d, want 0", s.Aggregated.TotalMutants)
	}
	if s.Aggregated.Statistics.Score != 0 {
		t.Fatalf("mutation score = %f, want 0", s.Aggregated.Statistics.Score)
	}
	if len(s.SurvivedTop) != 0 {
		t.Fatalf("survived top = %d, want 0", len(s.SurvivedTop))
	}

	out := formatTextSummary(s)
	if !strings.Contains(out, "Top Survived Mutants: none") {
		t.Fatalf("text summary missing zero-mutant marker: %q", out)
	}
}

func TestLoadReportWithNestedRuns(t *testing.T) {
	reportPath := writeTempReport(t, `{
  "totalFiles": 0,
  "processedFiles": 0,
  "totalMutants": 0,
  "killedMutants": 0,
  "results": [],
  "runs": [
    {
      "totalFiles": 1,
      "processedFiles": 1,
      "totalMutants": 1,
      "killedMutants": 1,
      "results": [
        {
          "mutant": {
            "filePath": "internal/a/a.go",
            "line": 3,
            "column": 1,
            "type": "conditional_binary",
            "description": "Replace == with !="
          },
          "status": "KILLED"
        }
      ],
      "statistics": {
        "killed": 1,
        "survived": 0,
        "timedOut": 0,
        "errors": 0,
        "notViable": 0,
        "mutationScore": 100
      }
    },
    {
      "totalFiles": 1,
      "processedFiles": 1,
      "totalMutants": 1,
      "killedMutants": 0,
      "results": [
        {
          "mutant": {
            "filePath": "internal/b/b.go",
            "line": 4,
            "column": 1,
            "type": "logical_binary",
            "description": "Replace && with ||"
          },
          "status": "SURVIVED"
        }
      ],
      "statistics": {
        "killed": 0,
        "survived": 1,
        "timedOut": 0,
        "errors": 0,
        "notViable": 0,
        "mutationScore": 0
      }
    }
  ]
}`)

	reports, err := loadReports([]string{reportPath})
	if err != nil {
		t.Fatalf("loadReports() error = %v", err)
	}
	if got := len(reports); got != 2 {
		t.Fatalf("len(reports) = %d, want 2", got)
	}
}
