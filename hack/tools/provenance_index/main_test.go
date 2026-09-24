package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseApprovalProvenance(t *testing.T) {
	dir := t.TempDir()
	var checksums strings.Builder
	for _, name := range []string{"operator-policy-approval.hcl", "operator-policy-approval-with-backup.hcl"} {
		contents := []byte("approval for " + name)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), contents, 0o600))
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(contents), name)
	}
	checksumsPath := filepath.Join(dir, "checksums.txt")
	require.NoError(t, os.WriteFile(checksumsPath, []byte(checksums.String()), 0o600))
	index, err := buildReleaseIndex(args{checksumsPath: checksumsPath})
	require.NoError(t, err)
	files := index["release_artifacts"].(map[string]any)["files"].([]map[string]any)
	require.Len(t, files, 3)
	for _, file := range files[1:] {
		require.Equal(t, true, file["included_in_checksums_txt"])
		require.Equal(t, file["sha256"], file["checksums_txt_sha256"])
	}
}

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
