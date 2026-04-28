package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2/types"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value cannot be empty")
	}
	*s = append(*s, value)
	return nil
}

type options struct {
	Reports           stringList
	MarkdownOut       string
	JSONOut           string
	TopSpecs          int
	Lane              string
	Selector          string
	KubernetesVersion string
	OpenBAOVersion    string
}

type summary struct {
	Lane                       string        `json:"lane,omitempty"`
	Selector                   string        `json:"selector,omitempty"`
	KubernetesVersion          string        `json:"kubernetesVersion,omitempty"`
	OpenBAOVersion             string        `json:"openbaoVersion,omitempty"`
	ReportCount                int           `json:"reportCount"`
	SuiteSucceeded             bool          `json:"suiteSucceeded"`
	TotalSpecs                 int           `json:"totalSpecs"`
	SpecsThatWillRun           int           `json:"specsThatWillRun"`
	Passed                     int           `json:"passed"`
	Failed                     int           `json:"failed"`
	Skipped                    int           `json:"skipped"`
	Pending                    int           `json:"pending"`
	Other                      int           `json:"other"`
	RunTime                    string        `json:"runTime"`
	RunTimeSeconds             float64       `json:"runTimeSeconds"`
	SpecialSuiteFailureReasons []string      `json:"specialSuiteFailureReasons,omitempty"`
	SlowestSpecs               []specSummary `json:"slowestSpecs,omitempty"`
	Failures                   []specSummary `json:"failures,omitempty"`
}

type specSummary struct {
	Name           string   `json:"name"`
	State          string   `json:"state"`
	RunTime        string   `json:"runTime"`
	RunTimeSeconds float64  `json:"runTimeSeconds"`
	Location       string   `json:"location,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	FailureMessage string   `json:"failureMessage,omitempty"`
}

func main() {
	opts, err := parseOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_report: %v\n", err)
		os.Exit(2)
	}

	reports, err := loadReports(opts.Reports)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e_report: %v\n", err)
		os.Exit(1)
	}

	s := buildSummary(reports, opts)

	if opts.JSONOut != "" {
		if err := writeJSON(opts.JSONOut, s); err != nil {
			fmt.Fprintf(os.Stderr, "e2e_report: %v\n", err)
			os.Exit(1)
		}
	}

	markdown := formatMarkdown(s)
	if opts.MarkdownOut != "" {
		if err := writeText(opts.MarkdownOut, markdown); err != nil {
			fmt.Fprintf(os.Stderr, "e2e_report: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Print(markdown)
}

func parseOptions() (options, error) {
	var opts options
	flag.Var(&opts.Reports, "json-report", "Ginkgo JSON report path. May be passed more than once.")
	flag.StringVar(&opts.MarkdownOut, "markdown-out", "", "optional path for Markdown summary output")
	flag.StringVar(&opts.JSONOut, "json-out", "", "optional path for machine-readable summary output")
	flag.IntVar(&opts.TopSpecs, "top", 10, "number of slowest specs to include")
	flag.StringVar(&opts.Lane, "lane", "", "CI lane name")
	flag.StringVar(&opts.Selector, "selector", "", "Ginkgo label selector")
	flag.StringVar(&opts.KubernetesVersion, "kubernetes-version", "", "Kubernetes node image or version")
	flag.StringVar(&opts.OpenBAOVersion, "openbao-version", "", "OpenBao image or version")
	flag.Parse()

	if len(opts.Reports) == 0 {
		return options{}, fmt.Errorf("at least one --json-report path is required")
	}
	if opts.TopSpecs < 0 {
		return options{}, fmt.Errorf("--top must be >= 0")
	}
	return opts, nil
}

func loadReports(paths []string) ([]types.Report, error) {
	var reports []types.Report
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read report %s: %w", path, err)
		}

		var decoded []types.Report
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, fmt.Errorf("parse report %s: %w", path, err)
		}
		reports = append(reports, decoded...)
	}
	if len(reports) == 0 {
		return nil, fmt.Errorf("no reports decoded")
	}
	return reports, nil
}

func buildSummary(reports []types.Report, opts options) summary {
	out := summary{
		Lane:              opts.Lane,
		Selector:          opts.Selector,
		KubernetesVersion: opts.KubernetesVersion,
		OpenBAOVersion:    opts.OpenBAOVersion,
		ReportCount:       len(reports),
		SuiteSucceeded:    true,
	}

	var specs []specSummary
	for _, report := range reports {
		out.SuiteSucceeded = out.SuiteSucceeded && report.SuiteSucceeded
		out.TotalSpecs += report.PreRunStats.TotalSpecs
		out.SpecsThatWillRun += report.PreRunStats.SpecsThatWillRun
		out.SpecialSuiteFailureReasons = append(out.SpecialSuiteFailureReasons, report.SpecialSuiteFailureReasons...)
		out.RunTimeSeconds += report.RunTime.Seconds()

		for _, spec := range report.SpecReports {
			specOut := summarizeSpec(spec)
			specs = append(specs, specOut)

			switch {
			case spec.State.Is(types.SpecStatePassed):
				out.Passed++
			case spec.State.Is(types.SpecStateFailureStates):
				out.Failed++
				out.Failures = append(out.Failures, specOut)
			case spec.State.Is(types.SpecStateSkipped):
				out.Skipped++
			case spec.State.Is(types.SpecStatePending):
				out.Pending++
			default:
				out.Other++
			}
		}
	}

	out.RunTimeSeconds = roundSeconds(out.RunTimeSeconds)
	out.RunTime = formatDuration(out.RunTimeSeconds)

	sort.SliceStable(specs, func(i, j int) bool {
		return specs[i].RunTimeSeconds > specs[j].RunTimeSeconds
	})
	if opts.TopSpecs > len(specs) {
		opts.TopSpecs = len(specs)
	}
	out.SlowestSpecs = append(out.SlowestSpecs, specs[:opts.TopSpecs]...)

	sort.SliceStable(out.Failures, func(i, j int) bool {
		return out.Failures[i].Name < out.Failures[j].Name
	})

	out.SpecialSuiteFailureReasons = uniqueStrings(out.SpecialSuiteFailureReasons)
	return out
}

func summarizeSpec(spec types.SpecReport) specSummary {
	return specSummary{
		Name:           specName(spec),
		State:          spec.State.String(),
		RunTime:        formatDuration(spec.RunTime.Seconds()),
		RunTimeSeconds: roundSeconds(spec.RunTime.Seconds()),
		Location:       spec.LeafNodeLocation.String(),
		Labels:         specLabels(spec),
		FailureMessage: firstLine(spec.Failure.Message),
	}
}

func specName(spec types.SpecReport) string {
	parts := append([]string{}, spec.ContainerHierarchyTexts...)
	if strings.TrimSpace(spec.LeafNodeText) != "" {
		parts = append(parts, spec.LeafNodeText)
	}

	filtered := parts[:0]
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, strings.TrimSpace(part))
		}
	}
	if len(filtered) == 0 {
		return "(suite node)"
	}
	return strings.Join(filtered, " ")
}

func specLabels(spec types.SpecReport) []string {
	seen := map[string]bool{}
	var labels []string
	for _, group := range spec.ContainerHierarchyLabels {
		for _, label := range group {
			if !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}
	for _, label := range spec.LeafNodeLabels {
		if !seen[label] {
			seen[label] = true
			labels = append(labels, label)
		}
	}
	sort.Strings(labels)
	return labels
}

func formatMarkdown(s summary) string {
	var b strings.Builder

	b.WriteString("## E2E Report Summary\n\n")
	b.WriteString("| Metric | Value |\n")
	b.WriteString("| --- | --- |\n")
	writeRow(&b, "Lane", valueOrUnset(s.Lane))
	writeRow(&b, "Selector", valueOrUnset(s.Selector))
	writeRow(&b, "Kubernetes", valueOrUnset(s.KubernetesVersion))
	writeRow(&b, "OpenBao", valueOrUnset(s.OpenBAOVersion))
	writeRow(&b, "Suite succeeded", fmt.Sprintf("%t", s.SuiteSucceeded))
	writeRow(&b, "Runtime", s.RunTime)
	writeRow(&b, "Reports", fmt.Sprintf("%d", s.ReportCount))
	writeRow(&b, "Total specs", fmt.Sprintf("%d", s.TotalSpecs))
	writeRow(&b, "Specs selected", fmt.Sprintf("%d", s.SpecsThatWillRun))
	writeRow(&b, "Passed", fmt.Sprintf("%d", s.Passed))
	writeRow(&b, "Failed", fmt.Sprintf("%d", s.Failed))
	writeRow(&b, "Skipped", fmt.Sprintf("%d", s.Skipped))
	writeRow(&b, "Pending", fmt.Sprintf("%d", s.Pending))
	if s.Other > 0 {
		writeRow(&b, "Other", fmt.Sprintf("%d", s.Other))
	}
	b.WriteString("\n")

	if len(s.SpecialSuiteFailureReasons) > 0 {
		b.WriteString("### Suite Failure Reasons\n\n")
		for _, reason := range s.SpecialSuiteFailureReasons {
			fmt.Fprintf(&b, "- %s\n", escapeMarkdown(reason))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Slowest Specs\n\n")
	if len(s.SlowestSpecs) == 0 {
		b.WriteString("No specs were reported.\n\n")
	} else {
		b.WriteString("| Runtime | State | Spec |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, spec := range s.SlowestSpecs {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", spec.RunTime, escapeMarkdown(spec.State), escapeMarkdown(spec.Name))
		}
		b.WriteString("\n")
	}

	if len(s.Failures) > 0 {
		b.WriteString("### Failures\n\n")
		b.WriteString("| State | Spec | Message |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, spec := range s.Failures {
			fmt.Fprintf(
				&b,
				"| %s | %s | %s |\n",
				escapeMarkdown(spec.State),
				escapeMarkdown(spec.Name),
				escapeMarkdown(valueOrUnset(spec.FailureMessage)),
			)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func writeRow(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "| %s | %s |\n", escapeMarkdown(key), escapeMarkdown(value))
}

func writeJSON(path string, s summary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o770); err != nil {
		return fmt.Errorf("create summary json dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary json: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o660); err != nil {
		return fmt.Errorf("write summary json: %w", err)
	}
	return nil
}

func writeText(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o770); err != nil {
		return fmt.Errorf("create summary markdown dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o660); err != nil {
		return fmt.Errorf("write summary markdown: %w", err)
	}
	return nil
}

func formatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	return (time.Duration(seconds*float64(time.Second)) / time.Millisecond).String()
}

func roundSeconds(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return value[:idx]
	}
	return value
}

func valueOrUnset(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(unset)"
	}
	return value
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func escapeMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
