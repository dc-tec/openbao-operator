package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
)

func TestBundleRevisionCannotBeOverwritten(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeBundles(dir, false))
	require.NoError(t, writeBundles(dir, true))
	path := filepath.Join(dir, configbuilder.OperatorPolicyBundleRevision, "rolling-update.hcl")
	require.NoError(t, os.WriteFile(path, []byte("previously published contents"), 0o644))
	require.ErrorContains(t, writeBundles(dir, false), "is immutable")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "previously published contents", string(data))
}
