package upgrade

import (
	"context"
	"fmt"

	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
	"github.com/go-logr/logr"
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
		return raftops.RunBlueGreenJoinGreenNonVoters(ctx, logger, cfg)
	case ExecutorActionBlueGreenWaitGreenSynced:
		return raftops.RunBlueGreenWaitGreenSynced(ctx, logger, cfg)
	case ExecutorActionBlueGreenPromoteGreenVoters:
		return raftops.RunBlueGreenPromoteGreenVoters(ctx, logger, cfg)
	case ExecutorActionBlueGreenDemoteBlueNonVotersStepDown:
		return raftops.RunBlueGreenDemoteBlueNonVotersStepDown(ctx, logger, cfg)
	case ExecutorActionBlueGreenRemoveBluePeers:
		return raftops.RunBlueGreenRemoveBluePeers(ctx, logger, cfg)
	case ExecutorActionBlueGreenRemoveGreenPeers:
		return raftops.RunBlueGreenRemoveGreenPeers(ctx, logger, cfg)
	case ExecutorActionBlueGreenRepairConsensus:
		return raftops.RunBlueGreenRepairConsensus(ctx, logger, cfg)
	case ExecutorActionRollingStepDownLeader:
		return raftops.RunRollingStepDownLeader(ctx, logger, cfg)
	default:
		return fmt.Errorf("unsupported action: %q", cfg.Action)
	}
}
