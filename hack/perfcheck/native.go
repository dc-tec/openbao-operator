package main

import (
	"context"

	testperf "github.com/dc-tec/openbao-operator/test/perf"
)

func runNativeScenario(
	ctx context.Context,
	opts options,
	cluster string,
	scenario scenarioSpec,
) (scenarioExecutionResult, error) {
	result, err := testperf.RunNativeScenario(ctx, nativePerfConfig(opts), cluster, testperf.Scenario{Name: scenario.Name})
	return scenarioExecutionResult{
		Phases:       nativePerfPhases(result.Phases),
		Measurements: result.Measurements,
		Namespace:    result.Namespace,
		Artifacts:    result.Artifacts,
		Cleanup:      result.Cleanup,
	}, err
}

func nativePerfConfig(opts options) testperf.Config {
	return testperf.Config{
		RunID:                  opts.RunID,
		ArtifactDir:            opts.ArtifactDir,
		ExistingClusterContext: opts.ExistingClusterContext,
		Namespace:              opts.Namespace,
		NamespacePrefix:        opts.NamespacePrefix,
		OperatorNS:             opts.OperatorNS,
		OpenBaoVersion:         opts.OpenBaoVersion,
		OpenBaoImage:           opts.OpenBaoImage,
		UpgradeFromVersion:     opts.UpgradeFromVersion,
		UpgradeFromImage:       opts.UpgradeFromImage,
		UpgradeToVersion:       opts.UpgradeToVersion,
		UpgradeToImage:         opts.UpgradeToImage,
		UpgradeExecutorImage:   opts.UpgradeExecutorImage,
		ConfigInitImage:        opts.ConfigInitImage,
		APIServerCIDR:          opts.APIServerCIDR,
		StorageClass:           opts.StorageClass,
		TenantChurnCount:       opts.TenantChurnCount,
	}
}

func nativePerfPhases(phases []testperf.Phase) []phaseEvent {
	if len(phases) == 0 {
		return nil
	}
	out := make([]phaseEvent, 0, len(phases))
	for _, phase := range phases {
		out = append(out, phaseEvent{
			Name:   phase.Name,
			At:     phase.At,
			Source: phase.Source,
		})
	}
	return out
}
