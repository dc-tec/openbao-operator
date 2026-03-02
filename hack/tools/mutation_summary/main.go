package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiStringFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	*m = append(*m, trimmed)
	return nil
}

type mutationReport struct {
	TotalFiles     int               `json:"totalFiles"`
	ProcessedFiles int               `json:"processedFiles"`
	TotalMutants   int               `json:"totalMutants"`
	KilledMutants  int               `json:"killedMutants"`
	Results        []mutationResult  `json:"results"`
	Statistics     reportStatistics  `json:"statistics"`
	Runs           []json.RawMessage `json:"runs,omitempty"`
	source         string
}

type mutationResult struct {
	Mutant mutationMutant `json:"mutant"`
	Status string         `json:"status"`
}

type mutationMutant struct {
	FilePath    string `json:"filePath"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type reportStatistics struct {
	Killed    int     `json:"killed"`
	Survived  int     `json:"survived"`
	TimedOut  int     `json:"timedOut"`
	Errors    int     `json:"errors"`
	NotViable int     `json:"notViable"`
	Score     float64 `json:"mutationScore"`
}

type aggregateReport struct {
	TotalFiles     int              `json:"totalFiles"`
	ProcessedFiles int              `json:"processedFiles"`
	TotalMutants   int              `json:"totalMutants"`
	KilledMutants  int              `json:"killedMutants"`
	Results        []mutationResult `json:"results"`
	Statistics     reportStatistics `json:"statistics"`
	SourceReports  []string         `json:"sourceReports"`
}

type survivedRow struct {
	FilePath    string
	Line        int
	Column      int
	Type        string
	Description string
	Count       int
}

type summary struct {
	ReportCount int
	Aggregated  aggregateReport
	SurvivedTop []survivedRow
}

func main() {
	var reportPaths multiStringFlag
	var top int
	var format string
	var writeJSON string
	var output string

	flag.Var(&reportPaths, "report", "Path to a mutation-report.json file (repeatable)")
	flag.IntVar(&top, "top", 10, "Maximum number of survived mutants to print")
	flag.StringVar(&format, "format", "text", "Summary output format: text or markdown")
	flag.StringVar(&writeJSON, "write-json", "", "Optional output path for merged mutation-report.json")
	flag.StringVar(&output, "output", "", "Optional output path for summary text")
	flag.Parse()

	if len(reportPaths) == 0 {
		reportPaths = append(reportPaths, "mutation-report.json")
	}

	reports, err := loadReports(reportPaths)
	if err != nil {
		fail(err)
	}

	if len(reports) == 0 {
		fail(fmt.Errorf("no valid reports were loaded"))
	}

	s := buildSummary(reports, top)
	text, err := formatSummary(s, format)
	if err != nil {
		fail(err)
	}

	if writeJSON != "" {
		if err := writeMergedReport(writeJSON, s.Aggregated); err != nil {
			fail(err)
		}
	}

	if output == "" {
		fmt.Print(text)
		return
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fail(fmt.Errorf("create output dir for %s: %w", output, err))
	}
	if err := os.WriteFile(output, []byte(text), 0o644); err != nil {
		fail(fmt.Errorf("write summary to %s: %w", output, err))
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "mutation_summary: %v\n", err)
	os.Exit(1)
}

func loadReports(paths []string) ([]mutationReport, error) {
	var reports []mutationReport
	for _, path := range paths {
		flattened, err := loadReport(path)
		if err != nil {
			return nil, err
		}
		reports = append(reports, flattened...)
	}
	return reports, nil
}

func loadReport(path string) ([]mutationReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read report %s: %w", path, err)
	}

	var parsed mutationReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse report %s: %w", path, err)
	}

	parsed.source = path

	// Support merged reports that contain nested `runs`.
	if len(parsed.Runs) > 0 && parsed.TotalMutants == 0 && len(parsed.Results) == 0 {
		var nested []mutationReport
		for idx, raw := range parsed.Runs {
			var run mutationReport
			if err := json.Unmarshal(raw, &run); err != nil {
				return nil, fmt.Errorf("parse nested run %d in %s: %w", idx, path, err)
			}
			run.source = fmt.Sprintf("%s[runs:%d]", path, idx)
			nested = append(nested, run)
		}
		return nested, nil
	}

	return []mutationReport{parsed}, nil
}

func buildSummary(reports []mutationReport, top int) summary {
	agg := aggregateReport{
		Results:       make([]mutationResult, 0),
		SourceReports: make([]string, 0, len(reports)),
	}

	survived := make(map[string]*survivedRow)

	for _, r := range reports {
		agg.SourceReports = append(agg.SourceReports, r.source)

		agg.TotalFiles += r.TotalFiles
		agg.ProcessedFiles += r.ProcessedFiles

		stats := deriveStatistics(r)
		reportTotal := r.TotalMutants
		if reportTotal <= 0 {
			reportTotal = stats.Killed + stats.Survived + stats.TimedOut + stats.Errors + stats.NotViable
		}
		agg.TotalMutants += reportTotal
		agg.KilledMutants += stats.Killed

		agg.Statistics.Killed += stats.Killed
		agg.Statistics.Survived += stats.Survived
		agg.Statistics.TimedOut += stats.TimedOut
		agg.Statistics.Errors += stats.Errors
		agg.Statistics.NotViable += stats.NotViable

		if len(r.Results) > 0 {
			agg.Results = append(agg.Results, r.Results...)
		}

		for _, res := range r.Results {
			if strings.ToUpper(strings.TrimSpace(res.Status)) != "SURVIVED" {
				continue
			}

			key := fmt.Sprintf("%s:%d:%d:%s:%s",
				res.Mutant.FilePath,
				res.Mutant.Line,
				res.Mutant.Column,
				res.Mutant.Type,
				res.Mutant.Description,
			)
			if _, exists := survived[key]; !exists {
				survived[key] = &survivedRow{
					FilePath:    res.Mutant.FilePath,
					Line:        res.Mutant.Line,
					Column:      res.Mutant.Column,
					Type:        res.Mutant.Type,
					Description: res.Mutant.Description,
					Count:       0,
				}
			}
			survived[key].Count++
		}
	}

	valid := agg.Statistics.Killed + agg.Statistics.Survived + agg.Statistics.TimedOut + agg.Statistics.Errors
	if valid > 0 {
		agg.Statistics.Score = (float64(agg.Statistics.Killed) / float64(valid)) * 100.0
	}

	rows := make([]survivedRow, 0, len(survived))
	for _, row := range survived {
		rows = append(rows, *row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		if rows[i].FilePath != rows[j].FilePath {
			return rows[i].FilePath < rows[j].FilePath
		}
		if rows[i].Line != rows[j].Line {
			return rows[i].Line < rows[j].Line
		}
		return rows[i].Column < rows[j].Column
	})

	if top > 0 && len(rows) > top {
		rows = rows[:top]
	}

	return summary{
		ReportCount: len(reports),
		Aggregated:  agg,
		SurvivedTop: rows,
	}
}

func deriveStatistics(r mutationReport) reportStatistics {
	if len(r.Results) == 0 {
		return r.Statistics
	}

	var stats reportStatistics
	for _, result := range r.Results {
		switch strings.ToUpper(strings.TrimSpace(result.Status)) {
		case "KILLED":
			stats.Killed++
		case "SURVIVED":
			stats.Survived++
		case "TIMED_OUT":
			stats.TimedOut++
		case "ERROR":
			stats.Errors++
		case "NOT_VIABLE":
			stats.NotViable++
		}
	}

	valid := stats.Killed + stats.Survived + stats.TimedOut + stats.Errors
	if valid > 0 {
		stats.Score = (float64(stats.Killed) / float64(valid)) * 100.0
	}
	return stats
}

func formatSummary(s summary, format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return formatTextSummary(s), nil
	case "markdown", "md":
		return formatMarkdownSummary(s), nil
	default:
		return "", fmt.Errorf("unsupported format %q (expected text or markdown)", format)
	}
}

func formatTextSummary(s summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Mutation Summary\n")
	fmt.Fprintf(&b, "================\n")
	fmt.Fprintf(&b, "Reports: %d\n", s.ReportCount)
	fmt.Fprintf(&b, "Mutation Score: %.2f%%\n", s.Aggregated.Statistics.Score)
	fmt.Fprintf(&b, "Total Files: %d\n", s.Aggregated.TotalFiles)
	fmt.Fprintf(&b, "Processed Files: %d\n", s.Aggregated.ProcessedFiles)
	fmt.Fprintf(&b, "Total Mutants: %d\n", s.Aggregated.TotalMutants)
	fmt.Fprintf(&b, "Killed: %d\n", s.Aggregated.Statistics.Killed)
	fmt.Fprintf(&b, "Survived: %d\n", s.Aggregated.Statistics.Survived)
	fmt.Fprintf(&b, "Timed Out: %d\n", s.Aggregated.Statistics.TimedOut)
	fmt.Fprintf(&b, "Errors: %d\n", s.Aggregated.Statistics.Errors)
	fmt.Fprintf(&b, "Not Viable: %d\n", s.Aggregated.Statistics.NotViable)

	if len(s.SurvivedTop) == 0 {
		fmt.Fprintf(&b, "\nTop Survived Mutants: none\n")
		return b.String()
	}

	fmt.Fprintf(&b, "\nTop Survived Mutants:\n")
	for i, row := range s.SurvivedTop {
		fmt.Fprintf(&b, "%d. %s:%d:%d [%s] %s (count=%d)\n",
			i+1,
			row.FilePath,
			row.Line,
			row.Column,
			row.Type,
			row.Description,
			row.Count,
		)
	}
	return b.String()
}

func formatMarkdownSummary(s summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Mutation Summary\n\n")
	fmt.Fprintf(&b, "- Reports: %d\n", s.ReportCount)
	fmt.Fprintf(&b, "- Mutation Score: %.2f%%\n", s.Aggregated.Statistics.Score)
	fmt.Fprintf(&b, "- Total Files: %d\n", s.Aggregated.TotalFiles)
	fmt.Fprintf(&b, "- Processed Files: %d\n", s.Aggregated.ProcessedFiles)
	fmt.Fprintf(&b, "- Total Mutants: %d\n", s.Aggregated.TotalMutants)
	fmt.Fprintf(&b, "- Killed: %d\n", s.Aggregated.Statistics.Killed)
	fmt.Fprintf(&b, "- Survived: %d\n", s.Aggregated.Statistics.Survived)
	fmt.Fprintf(&b, "- Timed Out: %d\n", s.Aggregated.Statistics.TimedOut)
	fmt.Fprintf(&b, "- Errors: %d\n", s.Aggregated.Statistics.Errors)
	fmt.Fprintf(&b, "- Not Viable: %d\n", s.Aggregated.Statistics.NotViable)

	if len(s.SurvivedTop) == 0 {
		fmt.Fprintf(&b, "\n### Top Survived Mutants\n\nNone.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "\n### Top Survived Mutants\n\n")
	fmt.Fprintf(&b, "| # | Location | Type | Description | Count |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|\n")
	for i, row := range s.SurvivedTop {
		location := fmt.Sprintf("%s:%d:%d", row.FilePath, row.Line, row.Column)
		fmt.Fprintf(&b, "| %d | `%s` | `%s` | %s | %d |\n",
			i+1,
			location,
			row.Type,
			escapePipes(row.Description),
			row.Count,
		)
	}

	return b.String()
}

func escapePipes(v string) string {
	return strings.ReplaceAll(v, "|", "\\|")
}

func writeMergedReport(path string, report aggregateReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create merged report dir for %s: %w", path, err)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal merged report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write merged report %s: %w", path, err)
	}
	return nil
}
