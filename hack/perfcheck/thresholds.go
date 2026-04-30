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
		policies := scenario.MetricPolicies
		if len(policies) == 0 {
			policies = defaultMetricPoliciesForKeys(metricKeys)
		}

		thresholds := make(map[string]float64, len(policies))
		normalizedPolicies := make(map[string]metricPolicySpec, len(policies))
		for key, policy := range policies {
			normalized := normalizeMetricPolicy(key, policy)
			normalizedPolicies[key] = normalized
			if normalized.Policy == metricPolicyIgnore {
				continue
			}
			if normalized.Policy == metricPolicyMustBeZero {
				thresholds[key] = 0
				if normalized.Threshold != nil {
					thresholds[key] = *normalized.Threshold
				}
				continue
			}
			if normalized.Threshold != nil {
				thresholds[key] = *normalized.Threshold
				continue
			}

			maxVal := scenario.MaxMetrics[key]
			multiplier := baseline.Multipliers.Max
			if normalized.Multiplier == metricMultiplierP95 {
				multiplier = baseline.Multipliers.P95
			}
			threshold := math.Ceil(maxVal * multiplier)
			if normalized.Floor != nil && threshold < *normalized.Floor {
				threshold = *normalized.Floor
			}
			thresholds[key] = threshold
		}
		out.Scenarios[scenarioName] = scenarioThresholds{
			LabelFilter:    scenario.LabelFilter,
			MetricPolicies: normalizedPolicies,
			Metrics:        thresholds,
		}
	}

	return out
}

type comparisonResult struct {
	Findings []string
	Warnings []string
}

func applyScenarioPolicy(thresholds scenarioThresholds, scenario scenarioSpec) scenarioThresholds {
	thresholds.LabelFilter = scenario.LabelFilter
	if len(scenario.MetricPolicies) == 0 {
		return thresholds
	}

	filteredMetrics := make(map[string]float64, len(scenario.MetricPolicies))
	for metric, policy := range scenario.MetricPolicies {
		normalized := normalizeMetricPolicy(metric, policy)
		if normalized.Policy == metricPolicyIgnore {
			continue
		}
		if threshold, ok := thresholds.Metrics[metric]; ok {
			filteredMetrics[metric] = threshold
		}
	}
	thresholds.Metrics = filteredMetrics
	thresholds.MetricPolicies = scenario.MetricPolicies
	return thresholds
}

func validateScenarioThresholds(thresholds scenarioThresholds, scenario scenarioSpec) error {
	for metric, policy := range scenario.MetricPolicies {
		normalized := normalizeMetricPolicy(metric, policy)
		if normalized.Policy == metricPolicyIgnore {
			continue
		}
		if _, ok := thresholds.Metrics[metric]; !ok {
			return fmt.Errorf("thresholds missing metric %q for scenario %q", metric, scenario.Name)
		}
	}
	return nil
}

func compareScenarioMetricsDetailed(
	scenarioName string,
	measured map[string]float64,
	thresholds scenarioThresholds,
) comparisonResult {
	result := comparisonResult{
		Findings: make([]string, 0),
		Warnings: make([]string, 0),
	}
	keys := make([]string, 0, len(thresholds.Metrics))
	for key := range thresholds.Metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		threshold := thresholds.Metrics[key]
		policy := normalizeMetricPolicy(key, thresholds.MetricPolicies[key])
		if policy.Policy == metricPolicyIgnore {
			continue
		}

		value, ok := measured[key]
		if !ok {
			msg := fmt.Sprintf("%s: missing measured metric %q", scenarioName, key)
			result = appendComparisonMessage(result, policy, msg)
			continue
		}
		if value > threshold {
			verb := "exceeded threshold"
			if policy.Policy == metricPolicyMustBeZero {
				verb = "must remain zero"
			}
			msg := fmt.Sprintf(
				"%s: metric %s %s (value=%.3f threshold=%.3f)",
				scenarioName,
				key,
				verb,
				value,
				threshold,
			)
			result = appendComparisonMessage(result, policy, msg)
		}
	}
	return result
}

func appendComparisonMessage(result comparisonResult, policy metricPolicySpec, msg string) comparisonResult {
	if policy.Severity == metricSeverityWarn {
		result.Warnings = append(result.Warnings, msg)
		return result
	}
	result.Findings = append(result.Findings, msg)
	return result
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
