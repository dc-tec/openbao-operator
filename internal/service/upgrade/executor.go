package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
)

const (
	leaderElectionWaitDuration      = 5 * time.Second
	defaultLeaderSearchMaxAttempts  = 10
	defaultLeaderSearchWaitInterval = 2 * time.Second
	defaultLeaderTransferMaxRetries = 10
	singleLeaderSearchAttempt       = 1
)

// RunExecutor runs the upgrade executor action.
func RunExecutor(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	logger = logger.WithValues(
		"action", cfg.Action,
		"cluster_namespace", cfg.ClusterNamespace,
		"cluster_name", cfg.ClusterName,
		"replicas", cfg.ClusterReplicas,
		"blue_revision", cfg.BlueRevision,
		"green_revision", cfg.GreenRevision,
	)
	logger.Info("Upgrade executor starting")

	switch cfg.Action {
	case ExecutorActionBlueGreenJoinGreenNonVoters:
		return runBlueGreenJoinGreenNonVoters(ctx, logger, cfg)
	case ExecutorActionBlueGreenWaitGreenSynced:
		return runBlueGreenWaitGreenSynced(ctx, logger, cfg)
	case ExecutorActionBlueGreenPromoteGreenVoters:
		return runBlueGreenPromoteGreenVoters(ctx, logger, cfg)
	case ExecutorActionBlueGreenDemoteBlueNonVotersStepDown:
		return runBlueGreenDemoteBlueNonVotersStepDown(ctx, logger, cfg)
	case ExecutorActionBlueGreenRemoveBluePeers:
		return runBlueGreenRemoveBluePeers(ctx, logger, cfg)
	case ExecutorActionBlueGreenRemoveGreenPeers:
		return runBlueGreenRemoveGreenPeers(ctx, logger, cfg)
	case ExecutorActionBlueGreenRepairConsensus:
		return runBlueGreenRepairConsensus(ctx, logger, cfg)
	case ExecutorActionRollingStepDownLeader:
		return runRollingStepDownLeader(ctx, logger, cfg)
	default:
		return fmt.Errorf("unsupported action: %q", cfg.Action)
	}
}
