package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
)

func runRollingStepDownLeader(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	leaderURL, err := findLeader(ctx, cfg, "")
	if err != nil {
		return fmt.Errorf("failed to find leader: %w", err)
	}
	logger.Info("Leader found", "leader_url", leaderURL)

	token, err := loginJWT(ctx, cfg, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := factory.NewWithToken(leaderURL, token)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	logger.Info("Stepping down leader")
	if err := client.StepDown(ctx); err != nil {
		return fmt.Errorf("failed to step down leader: %w", err)
	}

	logger.Info("Leader stepped down")
	return nil
}

func runBlueGreenJoinGreenNonVoters(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	blueLeaderURL, err := findLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		return fmt.Errorf("failed to find Blue leader: %w", err)
	}
	logger.Info("Blue leader found", "blue_leader_url", blueLeaderURL)

	token, err := loginJWT(ctx, cfg, blueLeaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		greenPodURL := podURL(cfg, cfg.GreenRevision, i)
		logger.V(1).Info("Joining Green pod as non-voter", "green_pod_url", greenPodURL)
		client, err := factory.NewWithToken(greenPodURL, token)
		if err != nil {
			return fmt.Errorf("failed to create client for Green pod %q: %w", greenPodURL, err)
		}
		if err := client.JoinRaftCluster(ctx, blueLeaderURL, true, true); err != nil {
			if classifyJoinError(err) == benignErrorClassificationBenign {
				logger.V(1).Info("Join reported benign error; continuing", "green_pod_url", greenPodURL, "error", err.Error())
				continue
			}
			return fmt.Errorf("failed to join Green pod %q as non-voter: %w", greenPodURL, err)
		}
	}

	logger.Info("All Green pods joined as non-voters")
	return nil
}

func runBlueGreenWaitGreenSynced(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	blueLeaderURL, err := findLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		return fmt.Errorf("failed to find Blue leader: %w", err)
	}
	logger.Info("Blue leader found", "blue_leader_url", blueLeaderURL)

	token, err := loginJWT(ctx, cfg, blueLeaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := factory.NewWithToken(blueLeaderURL, token)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	autopilotSupported := true
	if _, err := client.ReadRaftAutopilotState(ctx); err != nil {
		if errors.Is(err, openbao.ErrAutopilotNotAvailable) {
			autopilotSupported = false
		} else {
			return fmt.Errorf("failed to read raft autopilot state: %w", err)
		}
	}

	if !autopilotSupported {
		logger.Info("Raft Autopilot state endpoint not available; falling back to raft configuration presence checks")

		config, err := client.ReadRaftConfiguration(ctx)
		if err != nil {
			return fmt.Errorf("failed to read Raft configuration: %w", err)
		}

		missingGreen := countMissingGreenServers(cfg, config)
		if missingGreen > 0 {
			return fmt.Errorf("green pods are missing from raft configuration: missing=%d", missingGreen)
		}

		logger.Info("Green pods are present in raft configuration; proceeding without sync verification")
		return nil
	}

	nextProgressLog := time.Now()
	for {
		state, err := client.ReadRaftAutopilotState(ctx)
		if err != nil {
			return fmt.Errorf("failed to read raft autopilot state: %w", err)
		}

		targetIndex, ok := raftAutopilotLeaderLastIndex(state)
		if !ok {
			targetIndex = raftAutopilotMaxLastIndex(state)
			logger.V(1).Info("Unable to determine Raft leader index from autopilot state; using max last index",
				"leader_hint", state.Leader,
				"target_index", targetIndex,
			)
		}

		evaluation := evaluateGreenSyncFromAutopilot(cfg, state, targetIndex)
		for _, missingPodName := range evaluation.MissingPods {
			logger.V(1).Info("Green pod not found in autopilot state", "expected_pod_name", missingPodName)
		}
		for _, unhealthyServer := range evaluation.UnhealthyServers {
			// Note: Non-voters may report as unhealthy during sync since they haven't
			// fully replicated yet. We log this for visibility but only block on delta.
			logger.V(1).Info(
				"Green pod is not healthy in autopilot (expected for non-voters syncing)",
				"pod_name", unhealthyServer.PodName,
				"healthy", unhealthyServer.Server.Healthy,
				"status", unhealthyServer.Server.Status,
				"last_index", unhealthyServer.Server.LastIndex,
			)
		}

		if evaluation.AllSynced {
			logger.Info("Green pods are synced", "target_index", targetIndex, "sync_threshold", cfg.SyncThreshold)
			return nil
		}

		if time.Now().After(nextProgressLog) {
			logger.Info("Waiting for Green sync",
				"target_index", targetIndex,
				"max_delta", evaluation.MaxDelta,
				"sync_threshold", cfg.SyncThreshold,
				"missing_green", evaluation.MissingGreen,
				"unhealthy_green", evaluation.UnhealthyGreen,
				"autopilot_servers", autopilotServerDebugNames(state),
			)
			nextProgressLog = time.Now().Add(10 * time.Second)
		}

		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("timed out waiting for Green sync: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// runBlueGreenRepairConsensus repairs Raft consensus during rollback by ensuring
// that all Blue pods are configured as voters and all Green pods are configured
// as non-voters in a single reconciliation pass. This reduces the risk of leaving
// the cluster in a mixed or split configuration when rollback is triggered from
// late blue/green phases.
func runBlueGreenRepairConsensus(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	if cfg.BlueRevision == "" {
		return fmt.Errorf("blue revision is required for consensus repair")
	}
	if cfg.GreenRevision == "" {
		return fmt.Errorf("green revision is required for consensus repair")
	}

	// Prefer a Blue leader when repairing consensus, since Blue should remain
	// the authoritative cluster after rollback. If that fails, fall back to any
	// leader we can reach (including Green) to read the Raft configuration.
	leaderURL, err := findPreferredLeaderWithFallback(
		ctx,
		logger,
		cfg,
		cfg.BlueRevision,
		cfg.GreenRevision,
		"Blue",
		"Green",
	)
	if err != nil {
		return fmt.Errorf("failed to find leader for consensus repair: %w", err)
	}
	logger.Info("Leader found for consensus repair", "leader_url", leaderURL)

	token, err := loginJWT(ctx, cfg, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate for consensus repair: %w", err)
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := factory.NewWithToken(leaderURL, token)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client for consensus repair: %w", err)
	}

	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration for consensus repair: %w", err)
	}

	isBlueServer := func(nodeID, address string) bool {
		return raftServerMatchesRevision(nodeID, address, cfg.ClusterName, cfg.BlueRevision, cfg.ClusterReplicas)
	}

	isGreenServer := func(nodeID, address string) bool {
		return raftServerMatchesRevision(nodeID, address, cfg.ClusterName, cfg.GreenRevision, cfg.ClusterReplicas)
	}

	for _, server := range config.Config.Servers {
		if !isBlueServer(server.NodeID, server.Address) {
			continue
		}

		if server.Voter {
			logger.V(1).Info("Blue peer already voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
			continue
		}

		logger.Info("Promoting Blue peer to voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
		if err := client.PromoteRaftPeer(ctx, server.NodeID); err != nil {
			return fmt.Errorf("failed to promote Blue peer %q to voter during consensus repair: %w", server.NodeID, err)
		}
	}

	for _, server := range config.Config.Servers {
		if !isGreenServer(server.NodeID, server.Address) {
			continue
		}

		if !server.Voter {
			logger.V(1).Info("Green peer already non-voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
			continue
		}

		logger.Info("Demoting Green peer to non-voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
		if err := client.DemoteRaftPeer(ctx, server.NodeID); err != nil {
			if isBenignDemoteError(err) {
				logger.V(1).Info("Green peer already non-voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
				continue
			}
			return fmt.Errorf("failed to demote Green peer %q to non-voter during consensus repair: %w", server.NodeID, err)
		}
	}

	logger.Info("Consensus repair completed: Blue voters and Green non-voters enforced")
	return nil
}

func runBlueGreenPromoteGreenVoters(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	blueLeaderURL, err := findLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		return fmt.Errorf("failed to find Blue leader: %w", err)
	}
	logger.Info("Blue leader found", "blue_leader_url", blueLeaderURL)

	token, err := loginJWT(ctx, cfg, blueLeaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := factory.NewWithToken(blueLeaderURL, token)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration: %w", err)
	}

	voterStatus := make(map[string]bool)
	for _, server := range config.Config.Servers {
		voterStatus[server.NodeID] = server.Voter
		logger.V(1).Info("Raft server in config",
			"node_id", server.NodeID,
			"address", server.Address,
			"voter", server.Voter,
			"leader", server.Leader)
	}

	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := revisionPodName(cfg.ClusterName, cfg.GreenRevision, i)

		if isVoter, found := voterStatus[greenPodName]; found {
			if isVoter {
				logger.V(1).Info("Green pod is already a voter, skipping", "pod_name", greenPodName)
				continue
			}
			logger.V(1).Info("Green pod found as non-voter, will promote", "pod_name", greenPodName)
		} else {
			logger.Info("WARNING: Green pod not found in Raft config", "pod_name", greenPodName)
		}

		logger.V(1).Info("Promoting Green pod to voter", "pod_name", greenPodName)
		if err := client.PromoteRaftPeer(ctx, greenPodName); err != nil {
			return fmt.Errorf("failed to promote Green pod %q to voter: %w", greenPodName, err)
		}
	}

	logger.Info("Green pods promoted to voters")
	return nil
}

func runBlueGreenDemoteBlueNonVotersStepDown(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	leaderURL, err := findInitialLeader(ctx, logger, cfg)
	if err != nil {
		return err
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := ensureGreenLeaderBySteppingDownBlue(ctx, logger, cfg, factory, leaderURL)
	if err != nil {
		return err
	}

	if err := demoteAllBluePods(ctx, logger, cfg, client); err != nil {
		return err
	}

	logger.Info("Blue pods demoted to non-voters")
	return nil
}

func runBlueGreenRemoveBluePeers(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	return runBlueGreenRemovePeers(ctx, logger, cfg, cfg.BlueRevision, cfg.GreenRevision, cfg.BlueRevision, "Blue")
}

func runBlueGreenRemoveGreenPeers(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	return runBlueGreenRemovePeers(ctx, logger, cfg, cfg.GreenRevision, cfg.BlueRevision, cfg.GreenRevision, "Green")
}

func runBlueGreenRemovePeers(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	revisionToRemove string,
	preferredLeaderRevision string,
	fallbackLeaderRevision string,
	peerColor string,
) error {
	if strings.TrimSpace(revisionToRemove) == "" {
		return fmt.Errorf("revision to remove is required")
	}

	leaderURL, err := findPreferredLeaderWithFallback(
		ctx,
		logger,
		cfg,
		preferredLeaderRevision,
		fallbackLeaderRevision,
		"preferred",
		"fallback",
	)
	if err != nil {
		return fmt.Errorf("failed to find leader: %w", err)
	}
	logger.Info("Leader found", "leader_url", leaderURL, "target_revision", revisionToRemove, "target_peers", peerColor)

	token, err := loginJWT(ctx, cfg, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := factory.NewWithToken(leaderURL, token)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration: %w", err)
	}

	for _, server := range config.Config.Servers {
		if !raftServerMatchesRevision(server.NodeID, server.Address, cfg.ClusterName, revisionToRemove, cfg.ClusterReplicas) {
			continue
		}

		logger.Info("Removing Raft peer",
			"target", peerColor,
			"node_id", server.NodeID,
			"address", server.Address)
		if err := client.RemoveRaftPeer(ctx, server.NodeID); err != nil {
			return fmt.Errorf("failed to remove Raft peer %q: %w", server.NodeID, err)
		}
	}

	logger.Info("Raft peers removed", "target", peerColor)
	return nil
}
