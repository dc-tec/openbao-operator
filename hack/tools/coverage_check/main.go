package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var coverageLinePattern = regexp.MustCompile(
	`^(.+):([0-9]+)\.([0-9]+),([0-9]+)\.([0-9]+) ([0-9]+) ([0-9]+)$`,
)

type coverageStats struct {
	Covered int64
	Total   int64
}

func (s coverageStats) percent() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Covered) * 100 / float64(s.Total)
}

type coverageBlock struct {
	Layer   string
	Covered bool
	Total   int64
}

type coverageReport struct {
	Internal coverageStats
	Layers   map[string]coverageStats
}

func main() {
	profilePath := flag.String("profile", "cover.out", "Path to a Go coverage profile")
	minimum := flag.Float64("minimum", -1, "Minimum required internal package coverage percentage")
	flag.Parse()

	if math.IsNaN(*minimum) || math.IsInf(*minimum, 0) || *minimum < 0 || *minimum > 100 {
		fail(fmt.Errorf("minimum must be between 0 and 100"))
	}

	profile, err := os.Open(*profilePath)
	if err != nil {
		fail(fmt.Errorf("open profile %s: %w", *profilePath, err))
	}
	defer func() { _ = profile.Close() }()

	report, err := parseCoverageProfile(profile)
	if err != nil {
		fail(fmt.Errorf("parse profile %s: %w", *profilePath, err))
	}

	if err := printCoverageReport(os.Stdout, report, *minimum); err != nil {
		fail(fmt.Errorf("write report: %w", err))
	}
	if err := verifyMinimum(report, *minimum); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "coverage_check: %v\n", err)
	os.Exit(1)
}

func parseCoverageProfile(r io.Reader) (coverageReport, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return coverageReport{}, fmt.Errorf("read mode: %w", err)
		}
		return coverageReport{}, fmt.Errorf("profile is empty")
	}

	mode := strings.TrimPrefix(scanner.Text(), "mode: ")
	if mode != "set" && mode != "count" && mode != "atomic" {
		return coverageReport{}, fmt.Errorf("unsupported mode line %q", scanner.Text())
	}

	blocks := make(map[string]coverageBlock)
	lineNumber := 1
	for scanner.Scan() {
		lineNumber++
		matches := coverageLinePattern.FindStringSubmatch(scanner.Text())
		if matches == nil {
			return coverageReport{}, fmt.Errorf("line %d has invalid coverage syntax", lineNumber)
		}

		layer, ok := internalLayer(matches[1])
		if !ok {
			continue
		}

		total, err := strconv.ParseInt(matches[6], 10, 64)
		if err != nil {
			return coverageReport{}, fmt.Errorf("line %d statement count: %w", lineNumber, err)
		}
		count, err := strconv.ParseInt(matches[7], 10, 64)
		if err != nil {
			return coverageReport{}, fmt.Errorf("line %d execution count: %w", lineNumber, err)
		}

		key := strings.Join(matches[1:6], ":")
		if existing, found := blocks[key]; found {
			if existing.Total != total {
				return coverageReport{}, fmt.Errorf(
					"line %d changes statement count for duplicate block from %d to %d",
					lineNumber,
					existing.Total,
					total,
				)
			}
			existing.Covered = existing.Covered || count > 0
			blocks[key] = existing
			continue
		}

		blocks[key] = coverageBlock{
			Layer:   layer,
			Covered: count > 0,
			Total:   total,
		}
	}
	if err := scanner.Err(); err != nil {
		return coverageReport{}, fmt.Errorf("read profile: %w", err)
	}
	if len(blocks) == 0 {
		return coverageReport{}, fmt.Errorf("profile contains no internal package statements")
	}

	report := coverageReport{Layers: make(map[string]coverageStats)}
	for _, block := range blocks {
		report.Internal.Total += block.Total
		layerStats := report.Layers[block.Layer]
		layerStats.Total += block.Total
		if block.Covered {
			report.Internal.Covered += block.Total
			layerStats.Covered += block.Total
		}
		report.Layers[block.Layer] = layerStats
	}

	return report, nil
}

func internalLayer(fileName string) (string, bool) {
	normalized := strings.ReplaceAll(fileName, "\\", "/")
	const modulePrefix = "github.com/dc-tec/openbao-operator/internal/"

	var relative string
	if strings.HasPrefix(normalized, modulePrefix) {
		relative = strings.TrimPrefix(normalized, modulePrefix)
	} else if strings.HasPrefix(normalized, "internal/") {
		relative = strings.TrimPrefix(normalized, "internal/")
	} else {
		return "", false
	}

	layer, _, found := strings.Cut(relative, "/")
	if layer == "" {
		return "", false
	}
	if !found {
		return "root", true
	}
	return layer, true
}

func verifyMinimum(report coverageReport, minimum float64) error {
	actual := report.Internal.percent()
	if actual < minimum {
		return fmt.Errorf("internal coverage %.2f%% is below required minimum %.2f%%", actual, minimum)
	}
	return nil
}

func printCoverageReport(w io.Writer, report coverageReport, minimum float64) error {
	if _, err := fmt.Fprintf(
		w,
		"internal coverage: %.2f%% (%d/%d statements)\n",
		report.Internal.percent(),
		report.Internal.Covered,
		report.Internal.Total,
	); err != nil {
		return err
	}

	layers := make([]string, 0, len(report.Layers))
	for layer := range report.Layers {
		layers = append(layers, layer)
	}
	sort.Strings(layers)
	for _, layer := range layers {
		stats := report.Layers[layer]
		if _, err := fmt.Fprintf(
			w,
			"  %-12s %.2f%% (%d/%d)\n",
			layer,
			stats.percent(),
			stats.Covered,
			stats.Total,
		); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "required minimum: %.2f%%\n", minimum); err != nil {
		return err
	}
	if verifyMinimum(report, minimum) == nil {
		_, err := fmt.Fprintln(w, "coverage gate: pass")
		return err
	}
	_, err := fmt.Fprintln(w, "coverage gate: fail")
	return err
}
