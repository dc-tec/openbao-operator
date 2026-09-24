package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
)

func TestApprovalManifest(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, wantError string
	}{
		{name: "manual enrollment",
			manifest: "apiVersion: openbao.org/v1alpha1\nkind: OpenBaoCluster\nspec:\n  backup: {}\n"},
		{name: "wrong resource", manifest: "kind: OpenBaoRestore\n", wantError: "OpenBaoCluster manifest is required"},
		{name: "unknown field", manifest: "kind: OpenBaoCluster\nspec:\n  reconcilePolices: true\n",
			wantError: "decode cluster manifest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cluster.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.manifest), 0o600))
			var output bytes.Buffer
			err := run(path, "", &output)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, output.String(), "invalid input must not produce an approval")
				return
			}
			require.NoError(t, err)
			require.Contains(t, output.String(), `path "sys/policies/acl/openbao-operator-backup"`)
			require.Contains(t, output.String(), "allowed_parameters")
			require.NotContains(t, output.String(), `path "sys/policies/acl/openbao-operator-policy-approval"`)
		})
	}
}

func TestReleaseApprovals(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "release")
	require.ErrorContains(t, run("cluster.yaml", dir, &bytes.Buffer{}), "mutually exclusive")
	require.NoError(t, run("", dir, &bytes.Buffer{}))
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, backup := range []bool{false, true} {
		name := "operator-policy-approval.hcl"
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		if backup {
			name = "operator-policy-approval-with-backup.hcl"
			cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
		}
		contents, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		for _, strategy := range []openbaov1alpha1.UpdateStrategyType{
			openbaov1alpha1.UpdateStrategyRollingUpdate, openbaov1alpha1.UpdateStrategyBlueGreen,
		} {
			cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{Strategy: strategy}
			require.Equal(t, configbuilder.OperatorPolicyApproval(cluster), string(contents),
				"release artifact must match bootstrap approval for either strategy")
		}
	}
}
