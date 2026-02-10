package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

func buildThresholds(baseline baselineDocument) thresholdDocument {
	out := thresholdDocument{
		Version:      "v1",
		GeneratedAt:  time.Now().UTC(),
		NodeImage:    baseline.NodeImage,
		Multipliers:  baseline.Multipliers,
		MetricSchema: append([]string(nil), baseline.MetricSchema...),
		Scenarios:    make(map[string]scenarioThresholds, len(baseline.Scenarios)),
	}

	for scenarioName, scenario := range baseline.Scenarios {
		thresholds := make(map[string]float64, len(metricKeys))
		for _, key := range metricKeys {
			maxVal := scenario.MaxMetrics[key]
			multiplier := baseline.Multipliers.Max
			if _, ok := p95MetricSet[key]; ok {
				multiplier = baseline.Multipliers.P95
			}
			thresholds[key] = math.Ceil(maxVal * multiplier)
		}
		out.Scenarios[scenarioName] = scenarioThresholds{
			LabelFilter: scenario.LabelFilter,
			Metrics:     thresholds,
		}
	}

	return out
}

func compareScenarioMetrics(scenarioName string, measured map[string]float64, thresholds scenarioThresholds) []string {
	findings := make([]string, 0)
	keys := make([]string, 0, len(thresholds.Metrics))
	for key := range thresholds.Metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		threshold := thresholds.Metrics[key]
		value, ok := measured[key]
		if !ok {
			findings = append(findings, fmt.Sprintf("%s: missing measured metric %q", scenarioName, key))
			continue
		}
		if value > threshold {
			findings = append(findings, fmt.Sprintf(
				"%s: metric %s exceeded threshold (value=%.3f threshold=%.3f)",
				scenarioName,
				key,
				value,
				threshold,
			))
		}
	}
	return findings
}

func writeBaseline(path string, baseline baselineDocument) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create baseline directory: %w", err)
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

func writeThresholds(path string, thresholds thresholdDocument) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create thresholds directory: %w", err)
	}
	data, err := yaml.Marshal(thresholds)
	if err != nil {
		return fmt.Errorf("marshal thresholds: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write thresholds: %w", err)
	}
	return nil
}

func readThresholds(path string) (thresholdDocument, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return thresholdDocument{}, fmt.Errorf("read thresholds: %w", err)
	}
	var doc thresholdDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return thresholdDocument{}, fmt.Errorf("parse thresholds yaml: %w", err)
	}
	if doc.Version == "" {
		return thresholdDocument{}, fmt.Errorf("thresholds missing version")
	}
	if len(doc.Scenarios) == 0 {
		return thresholdDocument{}, fmt.Errorf("thresholds missing scenarios")
	}
	return doc, nil
}
