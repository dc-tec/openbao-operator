package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func writeSummaryArtifacts(opts options, summary runSummaryDocument) error {
	summaryOut := opts.SummaryOut
	if summaryOut == "" {
		summaryOut = filepath.Join(opts.ArtifactDir, "summary.json")
	}
	if err := writeJSONFile(summaryOut, summary); err != nil {
		return err
	}

	reportOut := opts.ReportOut
	if reportOut == "" {
		reportOut = filepath.Join(opts.ArtifactDir, "report.md")
	}
	if err := os.MkdirAll(filepath.Dir(reportOut), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := os.WriteFile(reportOut, []byte(renderMarkdownReport(summary)), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func renderMarkdownReport(summary runSummaryDocument) string {
	var b strings.Builder
	b.WriteString("# Perfcheck v2 Report\n\n")
	b.WriteString(fmt.Sprintf("- Generated: %s\n", summary.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	if summary.RunID != "" {
		b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", summary.RunID))
	}
	b.WriteString(fmt.Sprintf("- Artifacts: `%s`\n", summary.ArtifactDir))
	if summary.BaselineDir != "" {
		b.WriteString(fmt.Sprintf("- Baselines: `%s`\n", summary.BaselineDir))
	}
	if summary.PreviousRun != "" {
		b.WriteString(fmt.Sprintf("- Previous run: `%s`\n", summary.PreviousRun))
	}
	b.WriteString(fmt.Sprintf(
		"- Result: %d pass, %d warn, %d fail\n\n",
		summary.Totals.Pass,
		summary.Totals.Warn,
		summary.Totals.Fail,
	))

	names := make([]string, 0, len(summary.Scenarios))
	for name := range summary.Scenarios {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		scenario := summary.Scenarios[name]
		b.WriteString(fmt.Sprintf("## %s\n\n", name))
		b.WriteString(fmt.Sprintf("- Status: `%s`\n", scenario.Status))
		b.WriteString(fmt.Sprintf("- Samples: %d measured, %d warmup\n", scenario.Samples, scenario.Warmups))
		if len(scenario.Measurements) > 0 {
			b.WriteString("\n| Measurement | Median | Upper sample | Min | Max | N |\n")
			b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: |\n")
			measurementNames := make([]string, 0, len(scenario.Measurements))
			for measurement := range scenario.Measurements {
				measurementNames = append(measurementNames, measurement)
			}
			sort.Strings(measurementNames)
			for _, measurement := range measurementNames {
				s := scenario.Measurements[measurement]
				b.WriteString(fmt.Sprintf(
					"| `%s` | %.3f | %.3f | %.3f | %.3f | %d |\n",
					measurement,
					s.Median,
					s.UpperSample,
					s.Min,
					s.Max,
					s.Count,
				))
			}
		}
		if len(scenario.Findings) > 0 {
			b.WriteString("\nFindings:\n")
			for _, finding := range scenario.Findings {
				label := finding.Severity
				if finding.Measurement != "" {
					b.WriteString(fmt.Sprintf("- `%s` `%s`: %s\n", label, finding.Measurement, finding.Message))
				} else {
					b.WriteString(fmt.Sprintf("- `%s`: %s\n", label, finding.Message))
				}
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
