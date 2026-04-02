package upgrade

import "github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"

// ExecutorConfig holds the configuration for the upgrade executor Job.
type ExecutorConfig = raftops.ExecutorConfig

// LoadExecutorConfig loads the executor configuration from environment variables
// and mounted files.
func LoadExecutorConfig() (*ExecutorConfig, error) {
	return raftops.LoadExecutorConfig()
}
