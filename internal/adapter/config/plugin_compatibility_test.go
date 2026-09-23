package config

import (
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestRenderPluginCompatibility(t *testing.T) {
	digest := strings.Repeat("a", 64)
	tests := []struct {
		name      string
		version   string
		plugin    openbaov1alpha1.Plugin
		wantError string
	}{
		{name: "digest with embedded version", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Image: "registry.example.com/plugin:v1.0.0@sha256:" + digest}},
		{name: "digest with separate version", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Image: "registry.example.com/plugin@sha256:" + digest, Version: "v1.0.0"}},
		{name: "digest without version", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Image: "registry.example.com/plugin@sha256:" + digest}, wantError: "version is required"},
		{name: "tag without checksum", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Image: "registry.example.com/plugin:v1.0.0"}, wantError: "sha256sum is required"},
		{name: "tag with checksum", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Image: "registry.example.com/plugin:v1.0.0", SHA256Sum: digest}},
		{name: "command KMS", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Command: "plugin"}},
		{name: "command auth without version", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Type: "auth", Command: "plugin"}, wantError: "version is required"},
		{name: "absolute command", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Command: "/tmp/plugin"}, wantError: "relative to the plugin directory"},
		{name: "escaping command", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Command: "../plugin"}, wantError: "relative to the plugin directory"},
		{name: "both sources", version: "2.7.0", plugin: openbaov1alpha1.Plugin{Command: "plugin", Image: "registry.example.com/plugin:v1.0.0"}, wantError: "exactly one"},
		{name: "missing source", version: "2.7.0", wantError: "exactly one"},
		{name: "old version needs explicit fields", version: "2.6.3", plugin: openbaov1alpha1.Plugin{Command: "plugin"}, wantError: "required before OpenBao 2.7.0"},
		{name: "legacy complete declaration", version: "2.6.3", plugin: openbaov1alpha1.Plugin{Command: "plugin", Version: "v1.0.0", BinaryName: "plugin", SHA256Sum: digest}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalCluster("plugins", "default")
			cluster.Spec.Version = tt.version
			tt.plugin.Name = "example"
			if tt.plugin.Type == "" {
				tt.plugin.Type = "kms"
			}
			cluster.Spec.Plugins = []openbaov1alpha1.Plugin{tt.plugin}
			got, err := RenderHCL(cluster, InfrastructureDetails{Namespace: "default"})
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			for key, value := range map[string]string{"version": tt.plugin.Version, "binary_name": tt.plugin.BinaryName, "sha256sum": tt.plugin.SHA256Sum} {
				if value == "" {
					require.NotRegexp(t, `(?m)^\s*`+key+`\s*=`, string(got))
				}
			}
		})
	}
}

func TestRenderRemovedBuiltinSeal(t *testing.T) {
	cluster := newMinimalCluster("awskms", "default")
	cluster.Spec.Version = "2.7.0"
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: "awskms", AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{Region: "us-east-1", KMSKeyID: "test-key"}}
	_, err := RenderHCL(cluster, InfrastructureDetails{Namespace: "default"})
	require.ErrorContains(t, err, "install the external KMS plugin")
	cluster.Spec.Plugins = []openbaov1alpha1.Plugin{{Type: "kms", Name: "awskms", Command: "openbao-plugin-kms-awskms"}}
	got, err := RenderHCL(cluster, InfrastructureDetails{Namespace: "default"})
	require.NoError(t, err)
	require.Contains(t, string(got), `seal "awskms"`)
	require.Contains(t, string(got), `plugin "kms" "awskms"`)
	require.Contains(t, string(got), `kms_key_id = "test-key"`)
}
