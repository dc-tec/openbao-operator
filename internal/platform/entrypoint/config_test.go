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
			})
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
