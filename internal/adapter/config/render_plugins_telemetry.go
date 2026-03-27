package config

import (
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func buildPluginBlocks(plugins []openbaov1alpha1.Plugin) []*hclwrite.Block {
	blocks := make([]*hclwrite.Block, 0, len(plugins))
	for _, plugin := range plugins {
		if plugin.Type == "" || plugin.Name == "" {
			continue
		}

		var imagePtr *string
		var commandPtr *string
		if plugin.Image != "" {
			imagePtr = stringPtr(plugin.Image)
		} else if plugin.Command != "" {
			commandPtr = stringPtr(plugin.Command)
		}

		var argsPtr *[]string
		if len(plugin.Args) > 0 {
			args := append([]string(nil), plugin.Args...)
			argsPtr = &args
		}

		var envPtr *[]string
		if len(plugin.Env) > 0 {
			env := append([]string(nil), plugin.Env...)
			envPtr = &env
		}

		block := gohcl.EncodeAsBlock(hclPlugin{
			Type: plugin.Type,
			Name: plugin.Name,

			Image:   imagePtr,
			Command: commandPtr,

			Version:    plugin.Version,
			BinaryName: plugin.BinaryName,
			SHA256Sum:  plugin.SHA256Sum,

			Args: argsPtr,
			Env:  envPtr,
		}, "plugin")

		blocks = append(blocks, block)
	}
	return blocks
}

func buildTelemetryBlock(telemetry *openbaov1alpha1.TelemetryConfig) *hclwrite.Block {
	if telemetry == nil {
		return nil
	}

	var dogTagsPtr *[]string
	if len(telemetry.DogStatsdTags) > 0 {
		tags := append([]string(nil), telemetry.DogStatsdTags...)
		dogTagsPtr = &tags
	}

	return gohcl.EncodeAsBlock(hclTelemetry{
		UsageGaugePeriod:        stringPtr(telemetry.UsageGaugePeriod),
		MaximumGaugeCardinality: telemetry.MaximumGaugeCardinality,
		DisableHostname:         boolPtrTrue(telemetry.DisableHostname),
		EnableHostnameLabel:     boolPtrTrue(telemetry.EnableHostnameLabel),
		MetricsPrefix:           stringPtr(telemetry.MetricsPrefix),
		LeaseMetricsEpsilon:     stringPtr(telemetry.LeaseMetricsEpsilon),

		PrometheusRetentionTime: stringPtr(telemetry.PrometheusRetentionTime),

		StatsiteAddress: stringPtr(telemetry.StatsiteAddress),
		StatsdAddress:   stringPtr(telemetry.StatsdAddress),

		DogStatsdAddress: stringPtr(telemetry.DogStatsdAddress),
		DogStatsdTags:    dogTagsPtr,

		CirconusAPIKey:                     stringPtr(telemetry.CirconusAPIKey),
		CirconusAPIApp:                     stringPtr(telemetry.CirconusAPIApp),
		CirconusAPIURL:                     stringPtr(telemetry.CirconusAPIURL),
		CirconusSubmissionInterval:         stringPtr(telemetry.CirconusSubmissionInterval),
		CirconusCheckID:                    stringPtr(telemetry.CirconusCheckID),
		CirconusCheckForceMetricActivation: stringPtr(telemetry.CirconusCheckForceMetricActivation),
		CirconusCheckInstanceID:            stringPtr(telemetry.CirconusCheckInstanceID),
		CirconusCheckSearchTag:             stringPtr(telemetry.CirconusCheckSearchTag),
		CirconusCheckDisplayName:           stringPtr(telemetry.CirconusCheckDisplayName),
		CirconusCheckTags:                  stringPtr(telemetry.CirconusCheckTags),
		CirconusBrokerID:                   stringPtr(telemetry.CirconusBrokerID),
		CirconusBrokerSelectTag:            stringPtr(telemetry.CirconusBrokerSelectTag),

		StackdriverProjectID: stringPtr(telemetry.StackdriverProjectID),
		StackdriverLocation:  stringPtr(telemetry.StackdriverLocation),
		StackdriverNamespace: stringPtr(telemetry.StackdriverNamespace),
		StackdriverDebugLogs: boolPtrTrue(telemetry.StackdriverDebugLogs),
	}, "telemetry")
}
