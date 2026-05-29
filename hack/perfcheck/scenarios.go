package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func (d *yamlDuration) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Value == "" {
		return nil
	}
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", value.Value, err)
	}
	d.Duration = parsed
	d.set = true
	return nil
}

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
	if manifest.Version != versionV2 {
		return fmt.Errorf("scenario manifest version = %q, want %q", manifest.Version, versionV2)
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

		switch scenario.Executor {
		case executorE2EGinkgo:
			if strings.TrimSpace(scenario.LabelFilter) == "" {
				return fmt.Errorf("scenario %q uses %s but has no labelFilter", name, executorE2EGinkgo)
			}
		case executorNativeGo:
		case executorScript:
			if len(scenario.Command) == 0 {
				return fmt.Errorf("scenario %q uses %s but has no command", name, executorScript)
			}
		default:
			return fmt.Errorf("scenario %q has unsupported executor %q", name, scenario.Executor)
		}

		if len(scenario.Primary)+len(scenario.Diagnostic) == 0 {
			return fmt.Errorf("scenario %q has no measurements", name)
		}
		for _, phase := range scenario.Phases {
			if strings.TrimSpace(phase.Name) == "" {
				return fmt.Errorf("scenario %q contains phase with empty name", name)
			}
		}
		if scenario.Warmups != nil && *scenario.Warmups < 0 {
			return fmt.Errorf("scenario %q warmups must be >= 0", name)
		}
		if scenario.Samples != nil && *scenario.Samples <= 0 {
			return fmt.Errorf("scenario %q samples must be > 0", name)
		}
		if scenario.Cleanup != "" {
			switch scenario.Cleanup {
			case cleanupAlways, cleanupOnSuccess, cleanupNever:
			default:
				return fmt.Errorf("scenario %q has unsupported cleanup policy %q", name, scenario.Cleanup)
			}
		}
	}
	return nil
}

func selectedScenarios(opts options) ([]scenarioSpec, scenarioManifest, error) {
	manifest, err := loadScenarioManifest(opts.ScenarioPath)
	if err != nil {
		return nil, scenarioManifest{}, err
	}
	if len(opts.ScenarioNames) == 0 {
		return append([]scenarioSpec(nil), manifest.Scenarios...), manifest, nil
	}

	byName := scenarioMap(manifest.Scenarios)
	out := make([]scenarioSpec, 0, len(opts.ScenarioNames))
	for _, name := range opts.ScenarioNames {
		spec, ok := byName[name]
		if !ok {
			available := strings.Join(sortedScenarioNames(manifest.Scenarios), ", ")
			return nil, scenarioManifest{}, fmt.Errorf("unknown scenario %q (available: %s)", name, available)
		}
		out = append(out, spec)
	}
	return out, manifest, nil
}

func effectiveWarmups(opts options, manifest scenarioManifest, scenario scenarioSpec) int {
	if opts.WarmupsOverride >= 0 {
		return opts.WarmupsOverride
	}
	if scenario.Warmups != nil {
		return *scenario.Warmups
	}
	if manifest.Defaults.Warmups != nil {
		return *manifest.Defaults.Warmups
	}
	return 0
}

func effectiveSamples(opts options, manifest scenarioManifest, scenario scenarioSpec) int {
	if opts.SamplesOverride > 0 {
		return opts.SamplesOverride
	}
	if scenario.Samples != nil {
		return *scenario.Samples
	}
	if manifest.Defaults.Samples != nil {
		return *manifest.Defaults.Samples
	}
	return 1
}

func effectiveSampleTimeout(opts options, manifest scenarioManifest, scenario scenarioSpec) time.Duration {
	if opts.ScenarioTimeout > 0 {
		return opts.ScenarioTimeout
	}
	if scenario.SampleTimeout.set {
		return scenario.SampleTimeout.Duration
	}
	if manifest.Defaults.SampleTimeout.set {
		return manifest.Defaults.SampleTimeout.Duration
	}
	return 30 * time.Minute
}

func effectiveCleanup(manifest scenarioManifest, scenario scenarioSpec) string {
	if scenario.Cleanup != "" {
		return scenario.Cleanup
	}
	if manifest.Defaults.Cleanup != "" {
		return manifest.Defaults.Cleanup
	}
	return cleanupAlways
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

func scenarioRequiresExistingClusterSupport(opts options, scenario scenarioSpec) error {
	if opts.ExistingClusterContext == "" {
		return nil
	}
	if !scenario.ExistingCluster.Enabled {
		return fmt.Errorf("scenario %q does not declare existing-cluster support", scenario.Name)
	}
	if scenario.ExistingCluster.Destructive {
		return fmt.Errorf("scenario %q is destructive and cannot run against an existing cluster", scenario.Name)
	}
	if strings.TrimSpace(opts.Namespace) == "" && strings.TrimSpace(opts.NamespacePrefix) == "" {
		return fmt.Errorf("existing-cluster mode requires --namespace or --namespace-prefix")
	}
	return nil
}
