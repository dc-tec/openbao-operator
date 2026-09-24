package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
			err := run(path, &output)
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
