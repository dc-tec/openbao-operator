package backup

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// loadClusterConfig loads cluster namespace, name, and replicas from environment variables.
func loadClusterConfig(cfg *ExecutorConfig) error {
	cfg.ClusterNamespace = strings.TrimSpace(os.Getenv(constants.EnvClusterNamespace))
	if cfg.ClusterNamespace == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvClusterNamespace)
	}

	cfg.ClusterName = strings.TrimSpace(os.Getenv(constants.EnvClusterName))
	if cfg.ClusterName == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvClusterName)
	}

	replicasStr := strings.TrimSpace(os.Getenv(constants.EnvClusterReplicas))
	if replicasStr == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvClusterReplicas)
	}
	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid CLUSTER_REPLICAS value %q: %w", replicasStr, err)
	}
	cfg.ClusterReplicas = int32(replicas)

	cfg.StatefulSetName = strings.TrimSpace(os.Getenv(constants.EnvStatefulSetName))
	if cfg.StatefulSetName == "" {
		cfg.StatefulSetName = cfg.ClusterName
	}
	cfg.TLSServerName = strings.TrimSpace(os.Getenv(constants.EnvTLSServerName))
	return nil
}
