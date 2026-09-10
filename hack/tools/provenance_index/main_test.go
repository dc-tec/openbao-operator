package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelChartProvenance(t *testing.T) {
	t.Parallel()
	cfg := args{
		channel: "edge", repo: "dc-tec/openbao-operator", owner: "dc-tec",
		chartRef: "ghcr.io/dc-tec/charts-edge/openbao-operator", chartVersion: "0.5.0-edge.123.1.gaaaaaaaaaaaa",
		chartDigest: "sha256:" + strings.Repeat("a", 64), sourceRef: "refs/heads/main",
		checksumsSignerWorkflow: "dc-tec/openbao-operator/.github/workflows/publish-edge.yml",
	}
	index, err := buildChannelIndex(cfg)
	require.NoError(t, err)
	chart := index["chart"].(map[string]any)
	require.Equal(t, cfg.chartRef+"@"+cfg.chartDigest, chart["oci_subject"])
	require.Equal(t, cfg.chartVersion, chart["version"])
	require.Equal(t,
		"https://github.com/dc-tec/openbao-operator/.github/workflows/publish-edge.yml@refs/heads/main",
		chart["signing_identity"])
	require.Equal(t, cfg.checksumsSignerWorkflow, chart["attestation_signer_workflow"])
	cfg.chartVersion = ""
	_, err = buildChannelIndex(cfg)
	require.Error(t, err)
	cfg.chartDigest = ""
	cfg.channel = "nightly"
	index, err = buildChannelIndex(cfg)
	require.NoError(t, err)
	require.NotContains(t, index, "chart")
}
