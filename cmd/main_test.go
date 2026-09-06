package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandExitCodes(t *testing.T) {
	t.Setenv("OPERATOR_PLATFORM", "kubernetes")
	missingConfig := filepath.Join(t.TempDir(), "missing-kubeconfig")
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{name: "missing command", want: 2},
		{name: "unknown command", args: []string{"unknown"}, want: 2},
		{name: "top-level help", args: []string{"--help"}, want: 0},
		{name: "controller help", args: []string{"controller", "--help"}, want: 0},
		{name: "provisioner help", args: []string{"provisioner", "--help"}, want: 0},
		{name: "controller invalid flag", args: []string{"controller", "--unknown"}, want: 2},
		{name: "provisioner invalid flag", args: []string{"provisioner", "--unknown"}, want: 2},
		{name: "controller invalid config", args: []string{"controller", "--kubeconfig", missingConfig}, want: 1},
		{name: "provisioner invalid config", args: []string{"provisioner", "--kubeconfig", missingConfig}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := run(context.Background(), tc.args)
			require.Equal(t, tc.want, exitCode(err), "run error: %v", err)
		})
	}
	require.Zero(t, exitCode(nil))
	require.Equal(t, 1, exitCode(fmt.Errorf("manager failed")))
}
