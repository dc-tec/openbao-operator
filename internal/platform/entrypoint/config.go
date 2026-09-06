package entrypoint

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// UsageError identifies invalid command-line or environment configuration.
type UsageError struct {
	Err error
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// LoadConfig preserves controller-runtime's configuration precedence without
// reading the kubeconfig flag registered on the global command line.
func LoadConfig(kubeconfig string) (*rest.Config, error) {
	return loadConfig(kubeconfig, rest.InClusterConfig)
}

func loadConfig(kubeconfig string, inCluster func() (*rest.Config, error)) (*rest.Config, error) {
	if kubeconfig == "" && os.Getenv(clientcmd.RecommendedConfigPathEnvVar) == "" {
		if config, err := inCluster(); err == nil {
			config.QPS = -1
			return config, nil
		}
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = kubeconfig
	if kubeconfig == "" {
		if _, ok := os.LookupEnv("HOME"); !ok {
			currentUser, err := user.Current()
			if err != nil {
				return nil, fmt.Errorf("find home directory for kubeconfig: %w", err)
			}
			rules.Precedence = append(rules.Precedence,
				filepath.Join(currentUser.HomeDir, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName))
		}
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	// Match controller-runtime: API priority and fairness controls requests by default.
	if config.QPS == 0 {
		config.QPS = -1
	}
	return config, nil
}
