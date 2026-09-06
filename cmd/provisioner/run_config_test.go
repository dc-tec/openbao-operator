package provisioner

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
)

func TestParseRunConfig(t *testing.T) {
	originalArgs := append([]string(nil), os.Args...)
	originalFlags := flag.CommandLine
	for _, tc := range []struct {
		name      string
		args      []string
		wantError string
	}{
		{name: "unknown flag", args: []string{"--unknown"}, wantError: "flag provided but not defined"},
		{name: "invalid boolean", args: []string{"--admission-canary=perhaps"}, wantError: "invalid boolean value"},
		{name: "invalid duration", args: []string{"--admission-startup-timeout=later"}, wantError: "invalid value"},
		{name: "invalid admission mode", args: []string{"--admission-enforcement=off"}, wantError: "invalid admission"},
		{name: "positional argument", args: []string{"namespace"}, wantError: "unexpected positional argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRunConfig(tc.args, io.Discard)
			require.ErrorContains(t, err, tc.wantError)
			var usageError *entrypoint.UsageError
			require.ErrorAs(t, Run(context.Background(), tc.args), &usageError)
		})
	}
	for range 2 {
		cfg, err := parseRunConfig(
			[]string{"--admission-canary", "--admission-enforcement= Warn ", "--kubeconfig=/test/config"}, io.Discard,
		)
		require.NoError(t, err)
		require.True(t, cfg.admissionCanary)
		require.Equal(t, "warn", cfg.admissionEnforcement)
		require.Equal(t, "/test/config", cfg.kubeconfig)
		defaults, err := parseRunConfig(nil, io.Discard)
		require.NoError(t, err)
		require.False(t, defaults.admissionCanary)
		require.Equal(t, "fail", defaults.admissionEnforcement)
		require.Empty(t, defaults.kubeconfig)
	}
	require.Equal(t, originalArgs, os.Args)
	require.Same(t, originalFlags, flag.CommandLine)
	require.Nil(t, flag.CommandLine.Lookup("admission-canary"))
}

func TestParseRunConfigHelp(t *testing.T) {
	var output bytes.Buffer
	_, err := parseRunConfig([]string{"--help"}, &output)
	require.ErrorIs(t, err, flag.ErrHelp)
	require.Contains(t, output.String(), "Usage of provisioner:")
	require.Contains(t, output.String(), "-kubeconfig")
}
