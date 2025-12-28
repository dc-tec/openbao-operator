package upgrade

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/openbao/operator/internal/constants"
)

// ExecutorConfig holds the configuration for the upgrade executor Job.
type ExecutorConfig struct {
	ClusterNamespace string
	ClusterName      string
	ClusterReplicas  int32

	Action ExecutorAction

	JWTAuthRole string
	JWTToken    string

	TLSCACert []byte

	BlueRevision  string
	GreenRevision string

	SyncThreshold uint64
	Timeout       time.Duration
}

// Validate validates the executor configuration.
func (c *ExecutorConfig) Validate() error {
	if strings.TrimSpace(c.ClusterNamespace) == "" {
		return fmt.Errorf("cluster namespace is required")
	}
	if strings.TrimSpace(c.ClusterName) == "" {
		return fmt.Errorf("cluster name is required")
	}
	if c.ClusterReplicas <= 0 {
		return fmt.Errorf("cluster replicas must be greater than 0")
	}
	if strings.TrimSpace(string(c.Action)) == "" {
		return fmt.Errorf("upgrade action is required")
	}
	if strings.TrimSpace(c.JWTAuthRole) == "" {
		return fmt.Errorf("JWT auth role is required")
	}
	if strings.TrimSpace(c.JWTToken) == "" {
		return fmt.Errorf("JWT token is required")
	}
	if len(c.TLSCACert) == 0 {
		return fmt.Errorf("TLS CA certificate is required")
	}
	switch c.Action {
	case ExecutorActionBlueGreenJoinGreenNonVoters,
		ExecutorActionBlueGreenWaitGreenSynced,
		ExecutorActionBlueGreenPromoteGreenVoters,
		ExecutorActionBlueGreenDemoteBlueNonVotersStepDown,
		ExecutorActionBlueGreenRemoveBluePeers:
		if strings.TrimSpace(c.BlueRevision) == "" {
			return fmt.Errorf("blue revision is required for blue/green actions")
		}
		if strings.TrimSpace(c.GreenRevision) == "" {
			return fmt.Errorf("green revision is required for blue/green actions")
		}
	case ExecutorActionRollingStepDownLeader:
		// rolling step-down does not require revisions
	default:
		return fmt.Errorf("unknown upgrade action: %q", c.Action)
	}

	if c.SyncThreshold == 0 {
		return fmt.Errorf("sync threshold must be greater than 0")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}

	return nil
}

// LoadExecutorConfig loads the executor configuration from environment variables and mounted files.
func LoadExecutorConfig() (*ExecutorConfig, error) {
	cfg := &ExecutorConfig{
		SyncThreshold: 100,
		Timeout:       10 * time.Minute,
	}

	cfg.ClusterNamespace = strings.TrimSpace(os.Getenv(constants.EnvClusterNamespace))
	if cfg.ClusterNamespace == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvClusterNamespace)
	}
	cfg.ClusterName = strings.TrimSpace(os.Getenv(constants.EnvClusterName))
	if cfg.ClusterName == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvClusterName)
	}

	replicasStr := strings.TrimSpace(os.Getenv(constants.EnvClusterReplicas))
	if replicasStr == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvClusterReplicas)
	}
	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q: %w", constants.EnvClusterReplicas, replicasStr, err)
	}
	cfg.ClusterReplicas = int32(replicas)

	cfg.Action = ExecutorAction(strings.TrimSpace(os.Getenv(constants.EnvUpgradeAction)))
	if cfg.Action == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvUpgradeAction)
	}

	cfg.JWTAuthRole = strings.TrimSpace(os.Getenv(constants.EnvUpgradeJWTAuthRole))
	if cfg.JWTAuthRole == "" {
		return nil, fmt.Errorf("%s environment variable is required", constants.EnvUpgradeJWTAuthRole)
	}

	jwtTokenPath := constants.PathUpgradeJWTToken
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvJWTTokenPath)); envPath != "" {
		jwtTokenPath = envPath
	}
	jwtToken, err := os.ReadFile(jwtTokenPath) //#nosec G304 -- Path from constant or environment variable
	if err != nil {
		return nil, fmt.Errorf("failed to read JWT token from %q: %w", jwtTokenPath, err)
	}
	cfg.JWTToken = strings.TrimSpace(string(jwtToken))

	caCertPath := constants.PathTLSCACert
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvTLSCAPath)); envPath != "" {
		caCertPath = envPath
	}
	caCert, err := os.ReadFile(caCertPath) //#nosec G304 -- Path from constant or environment variable
	if err != nil {
		return nil, fmt.Errorf("failed to read TLS CA certificate from %q: %w", caCertPath, err)
	}
	cfg.TLSCACert = caCert

	cfg.BlueRevision = strings.TrimSpace(os.Getenv(constants.EnvUpgradeBlueRevision))
	cfg.GreenRevision = strings.TrimSpace(os.Getenv(constants.EnvUpgradeGreenRevision))

	if thresholdStr := strings.TrimSpace(os.Getenv(constants.EnvUpgradeSyncThreshold)); thresholdStr != "" {
		threshold, err := strconv.ParseUint(thresholdStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q: %w", constants.EnvUpgradeSyncThreshold, thresholdStr, err)
		}
		cfg.SyncThreshold = threshold
	}

	if timeoutStr := strings.TrimSpace(os.Getenv(constants.EnvUpgradeTimeout)); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value %q: %w", constants.EnvUpgradeTimeout, timeoutStr, err)
		}
		cfg.Timeout = timeout
	}

	return cfg, nil
}
