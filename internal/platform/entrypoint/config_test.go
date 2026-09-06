package entrypoint

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestLoadConfigPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeConfig := func(name string) string {
		path := filepath.Join(dir, name)
		config := clientcmdapi.Config{
			Clusters:       map[string]*clientcmdapi.Cluster{"test": {Server: "https://" + name + ".invalid"}},
			AuthInfos:      map[string]*clientcmdapi.AuthInfo{"test": {}},
			Contexts:       map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}},
			CurrentContext: "test",
		}
		require.NoError(t, clientcmd.WriteToFile(config, path))
		return path
	}
	explicit := writeConfig("explicit")
	environment := writeConfig("environment")
	for _, tc := range []struct {
		name, explicitPath, envPath, wantHost string
		wantInCluster                         bool
	}{
		{name: "explicit wins", explicitPath: explicit, envPath: environment, wantHost: "https://explicit.invalid"},
		{name: "environment before in-cluster", envPath: environment, wantHost: "https://environment.invalid"},
		{name: "in-cluster before default file", wantHost: "https://in-cluster.invalid", wantInCluster: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(clientcmd.RecommendedConfigPathEnvVar, tc.envPath)
			called := false
			config, err := loadConfig(tc.explicitPath, func() (*rest.Config, error) {
				called = true
				return &rest.Config{Host: "https://in-cluster.invalid"}, nil
			}, clientcmd.NewDefaultClientConfigLoadingRules)
			require.NoError(t, err)
			require.Equal(t, tc.wantHost, config.Host)
			require.Equal(t, float32(-1), config.QPS)
			require.Equal(t, tc.wantInCluster, called)
		})
	}
}

func TestLoadConfigDoesNotFallbackFromExplicitError(t *testing.T) {
	t.Setenv(clientcmd.RecommendedConfigPathEnvVar, filepath.Join(t.TempDir(), "environment"))
	path := filepath.Join(t.TempDir(), "missing")
	_, err := LoadConfig(path)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, path)
}

func TestUsageErrorPreservesCause(t *testing.T) {
	cause := fmt.Errorf("invalid option")
	err := &UsageError{Err: cause}
	require.ErrorIs(t, err, cause)
	require.Equal(t, cause.Error(), err.Error())
}

func TestExplicitConfigDoesNotMigrateDefaultFiles(t *testing.T) {
	for _, blockedDestination := range []bool{false, true} {
		t.Run(fmt.Sprintf("blocked destination %t", blockedDestination), func(t *testing.T) {
			dir := t.TempDir()
			explicit := filepath.Join(dir, "explicit")
			legacy := filepath.Join(dir, "legacy")
			destination := filepath.Join(dir, "default")
			config := clientcmdapi.Config{
				Clusters:       map[string]*clientcmdapi.Cluster{"test": {Server: "https://explicit.invalid"}},
				AuthInfos:      map[string]*clientcmdapi.AuthInfo{"test": {}},
				Contexts:       map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}},
				CurrentContext: "test",
			}
			require.NoError(t, clientcmd.WriteToFile(config, explicit))
			require.NoError(t, os.WriteFile(legacy, []byte("legacy config"), 0o600))
			if blockedDestination {
				parent := filepath.Join(dir, "blocked-parent")
				require.NoError(t, os.WriteFile(parent, []byte("not a directory"), 0o600))
				destination = filepath.Join(parent, "default")
			}
			loaded, err := loadConfig(explicit, func() (*rest.Config, error) {
				t.Fatal("explicit config must not consult in-cluster config")
				return nil, nil
			}, func() *clientcmd.ClientConfigLoadingRules {
				return &clientcmd.ClientConfigLoadingRules{
					MigrationRules: map[string]string{destination: legacy},
				}
			})
			require.NoError(t, err, "default-file migration must not interfere with explicit config")
			require.Equal(t, "https://explicit.invalid", loaded.Host)
			if !blockedDestination {
				require.NoFileExists(t, destination, "explicit config must not migrate default files")
			}
			contents, err := os.ReadFile(legacy)
			require.NoError(t, err)
			require.Equal(t, "legacy config", string(contents))
		})
	}
}
