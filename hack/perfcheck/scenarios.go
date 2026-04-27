package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultScenarioPath = "hack/perf/scenarios.yaml"

func loadScenarioManifest(path string) (scenarioManifest, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultScenarioPath
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return scenarioManifest{}, fmt.Errorf("read scenario manifest: %w", err)
	}

	var manifest scenarioManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return scenarioManifest{}, fmt.Errorf("parse scenario manifest: %w", err)
	}
	if err := validateScenarioManifest(manifest); err != nil {
		return scenarioManifest{}, err
	}
	return manifest, nil
}

func validateScenarioManifest(manifest scenarioManifest) error {
	if manifest.Version == "" {
		return fmt.Errorf("scenario manifest missing version")
	}
	if len(manifest.Scenarios) == 0 {
		return fmt.Errorf("scenario manifest missing scenarios")
	}

	seen := make(map[string]struct{}, len(manifest.Scenarios))
	for _, scenario := range manifest.Scenarios {
		name := strings.TrimSpace(scenario.Name)
		if name == "" {
			return fmt.Errorf("scenario manifest contains scenario with empty name")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("scenario manifest contains duplicate scenario %q", name)
		}
		seen[name] = struct{}{}

		if strings.TrimSpace(scenario.LabelFilter) == "" {
			return fmt.Errorf("scenario %q missing labelFilter", name)
		}
		if len(scenario.MetricPolicies) == 0 {
			return fmt.Errorf("scenario %q missing metricPolicies", name)
		}
		for metric, policy := range scenario.MetricPolicies {
			if err := validateMetricPolicy(metric, policy); err != nil {
				return fmt.Errorf("scenario %q metric %q: %w", name, metric, err)
			}
		}
	}
	return nil
}

func validateMetricPolicy(metric string, policy metricPolicySpec) error {
	if _, ok := metricKeySet()[metric]; !ok {
		return fmt.Errorf("unknown metric")
	}

	normalized := normalizeMetricPolicy(metric, policy)
	switch normalized.Policy {
	case metricPolicyUpperBound, metricPolicyMustBeZero, metricPolicyIgnore:
	default:
		return fmt.Errorf("unknown policy %q", policy.Policy)
	}

	switch normalized.Severity {
	case metricSeverityFail, metricSeverityWarn:
	default:
		return fmt.Errorf("unknown severity %q", policy.Severity)
	}

	switch normalized.Multiplier {
	case metricMultiplierP95, metricMultiplierMax:
	default:
		return fmt.Errorf("unknown multiplier %q", policy.Multiplier)
	}

	if normalized.Floor != nil && *normalized.Floor < 0 {
		return fmt.Errorf("floor must be >= 0")
	}
	if normalized.Threshold != nil && *normalized.Threshold < 0 {
		return fmt.Errorf("threshold must be >= 0")
	}
	return nil
}

func normalizeMetricPolicy(metric string, policy metricPolicySpec) metricPolicySpec {
	if strings.TrimSpace(policy.Policy) == "" {
		policy.Policy = metricPolicyUpperBound
	}
	if strings.TrimSpace(policy.Severity) == "" {
		policy.Severity = metricSeverityFail
	}
	if strings.TrimSpace(policy.Multiplier) == "" {
		if _, ok := p95MetricSet[metric]; ok {
			policy.Multiplier = metricMultiplierP95
		} else {
			policy.Multiplier = metricMultiplierMax
		}
	}
	return policy
}

func metricKeySet() map[string]struct{} {
	out := make(map[string]struct{}, len(metricKeys))
	for _, key := range metricKeys {
		out[key] = struct{}{}
	}
	return out
}

func defaultMetricPoliciesForKeys(keys []string) map[string]metricPolicySpec {
	out := make(map[string]metricPolicySpec, len(keys))
	for _, key := range keys {
		out[key] = normalizeMetricPolicy(key, metricPolicySpec{})
	}
	return out
}

func scenarioMap(scenarios []scenarioSpec) map[string]scenarioSpec {
	out := make(map[string]scenarioSpec, len(scenarios))
	for _, scenario := range scenarios {
		out[scenario.Name] = scenario
	}
	return out
}

func sortedScenarioNames(scenarios []scenarioSpec) []string {
	names := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		names = append(names, scenario.Name)
	}
	sort.Strings(names)
	return names
}
