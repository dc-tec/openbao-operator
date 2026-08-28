package raftops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/go-logr/logr"
)

// RunRollingStepDownLeader steps down the current cluster leader.
func RunRollingStepDownLeader(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	resolveClient := func(ctx context.Context, leaderURL string) (LeaderTransferClient, error) {
		client, err := NewAuthenticatedClient(ctx, cfg, factory, leaderURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
		}
		return client, nil
	}

	return runRollingStepDownLeaderWithFuncs(ctx, logger, cfg, RetryPolicy{MaxAttempts: defaultLeaderTransferMaxRetries}, FindLeader, resolveClient, WaitForNewLeaderURL)
}

func runRollingStepDownLeaderWithFuncs(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	policy RetryPolicy,
	findLeader func(context.Context, *ExecutorConfig, string) (string, error),
	resolveClient LeaderTransferClientResolver,
	waitForLeader LeaderTransferWaitFunc,
) error {
	policy = NormalizeLeaderTransferRetryPolicy(policy)

	for _, attempt := range AttemptOrdinals(policy.MaxAttempts) {
		attemptNumber := attempt + 1

		leaderURL, err := findLeader(ctx, cfg, cfg.BlueRevision)
		if err != nil {
			return fmt.Errorf("failed to find leader: %w", err)
		}
		logger.Info("Leader found", "leader_url", leaderURL, "attempt", attemptNumber, "max_attempts", policy.MaxAttempts)

		client, err := resolveClient(ctx, leaderURL)
		if err != nil {
			return err
		}

		logger.Info("Stepping down leader", "attempt", attemptNumber, "max_attempts", policy.MaxAttempts)
		classification, err := StepDownLeader(ctx, logger, client)
		if err != nil {
			if classification == BenignErrorClassificationFatal {
				return fmt.Errorf("failed to step down leader: %w", err)
			}
			logger.Info("Retryable leader step-down error observed; retrying", "attempt", attemptNumber, "max_attempts", policy.MaxAttempts, "error", err.Error())
			continue
		}

		nextLeaderURL, err := waitForLeader(ctx, logger, cfg, leaderURL)
		if err != nil {
			return fmt.Errorf("failed to wait for a new leader after step-down: %w", err)
		}
		if strings.TrimSpace(nextLeaderURL) == "" {
			logger.Info("Leader transfer did not surface a leader URL yet; retrying step-down", "attempt", attemptNumber, "max_attempts", policy.MaxAttempts)
			continue
		}
		if nextLeaderURL == leaderURL {
			logger.Info("Leader transferred back to the same node after step-down; retrying", "leader_url", leaderURL, "attempt", attemptNumber, "max_attempts", policy.MaxAttempts)
			continue
		}

		logger.Info("Leader transferred successfully", "previous_leader_url", leaderURL, "new_leader_url", nextLeaderURL, "attempt", attemptNumber, "max_attempts", policy.MaxAttempts)
		return nil
	}

	return fmt.Errorf("leader step-down did not transfer leadership after %d attempts", policy.MaxAttempts)
}

// RunBlueGreenJoinGreenNonVoters joins Green peers as non-voters under the Blue
// leader.
func RunBlueGreenJoinGreenNonVoters(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	blueLeaderURL, err := FindLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		return fmt.Errorf("failed to find Blue leader: %w", err)
	}
	logger.Info("Blue leader found", "blue_leader_url", blueLeaderURL)

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	token, err := LoginJWTIfStandard(ctx, cfg, factory, blueLeaderURL)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	for _, i := range ReplicaOrdinals(cfg.ClusterReplicas) {
		greenPodURL := PodURL(cfg, cfg.GreenRevision, i)
		logger.V(1).Info("Joining Green pod as non-voter", "green_pod_url", greenPodURL)
		client, err := NewAuthenticatedClientWithToken(ctx, cfg, factory, greenPodURL, token)
		if err != nil {
			return fmt.Errorf("failed to create client for Green pod %q: %w", greenPodURL, err)
		}
		if err := client.JoinRaftCluster(ctx, blueLeaderURL, true, true); err != nil {
			if ClassifyJoinError(err) == BenignErrorClassificationBenign {
				logger.V(1).Info("Join reported benign error; continuing", "green_pod_url", greenPodURL, "error", err.Error())
				continue
			}
			return fmt.Errorf("failed to join Green pod %q as non-voter: %w", greenPodURL, err)
		}
	}

	logger.Info("All Green pods joined as non-voters")
	return nil
}

// RunBlueGreenWaitGreenSynced waits until Green peers are present and, when
// possible, sufficiently replicated.
func RunBlueGreenWaitGreenSynced(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	blueLeaderURL, err := FindLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		return fmt.Errorf("failed to find Blue leader: %w", err)
	}
	logger.Info("Blue leader found", "blue_leader_url", blueLeaderURL)

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := NewAuthenticatedClient(ctx, cfg, factory, blueLeaderURL)
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

		missingGreen := CountMissingGreenServers(cfg, config)
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

		targetIndex, ok := RaftAutopilotLeaderLastIndex(state)
		if !ok {
			targetIndex = RaftAutopilotMaxLastIndex(state)
			logger.V(1).Info(
				"Unable to determine Raft leader index from autopilot state; using max last index",
				"leader_hint", state.Leader,
				"target_index", targetIndex,
			)
		}

		evaluation := EvaluateGreenSyncFromAutopilot(cfg, state, targetIndex)
		for _, missingPodName := range evaluation.MissingPods {
			logger.V(1).Info("Green pod not found in autopilot state", "expected_pod_name", missingPodName)
		}
		for _, unhealthyServer := range evaluation.UnhealthyServers {
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
			logger.Info(
				"Waiting for Green sync",
				"target_index", targetIndex,
				"max_delta", evaluation.MaxDelta,
				"sync_threshold", cfg.SyncThreshold,
				"missing_green", evaluation.MissingGreen,
				"unhealthy_green", evaluation.UnhealthyGreen,
				"autopilot_servers", AutopilotServerDebugNames(state),
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

// RunBlueGreenRepairConsensus enforces Blue voters and Green non-voters during
// rollback repair.
func RunBlueGreenRepairConsensus(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	if cfg.GreenRevision == "" {
		return fmt.Errorf("green revision is required for consensus repair")
	}

	leaderURL, err := FindPreferredLeaderWithFallback(
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

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := NewAuthenticatedClient(ctx, cfg, factory, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client for consensus repair: %w", err)
	}

	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration for consensus repair: %w", err)
	}

	isBlueServer := func(nodeID string, address string) bool {
		return RaftServerMatchesRevision(nodeID, address, cfg.ClusterName, cfg.BlueRevision, cfg.ClusterReplicas)
	}
	isGreenServer := func(nodeID string, address string) bool {
		return RaftServerMatchesRevision(nodeID, address, cfg.ClusterName, cfg.GreenRevision, cfg.ClusterReplicas)
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
		alreadyVoter, err := promoteRaftPeerAndVerify(ctx, client, server.NodeID)
		if err != nil {
			return fmt.Errorf("failed to promote Blue peer %q to voter during consensus repair: %w", server.NodeID, err)
		}
		if alreadyVoter {
			logger.V(1).Info("Blue peer already voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
			continue
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
			if IsBenignDemoteError(err) {
				logger.V(1).Info("Green peer already non-voter during consensus repair", "node_id", server.NodeID, "address", server.Address)
				continue
			}
			return fmt.Errorf("failed to demote Green peer %q to non-voter during consensus repair: %w", server.NodeID, err)
		}
	}

	logger.Info("Consensus repair completed: Blue voters and Green non-voters enforced")
	return nil
}

// RunBlueGreenPromoteGreenVoters promotes Green peers to voters under the Blue
// leader.
func RunBlueGreenPromoteGreenVoters(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	blueLeaderURL, err := FindLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		return fmt.Errorf("failed to find Blue leader: %w", err)
	}
	logger.Info("Blue leader found", "blue_leader_url", blueLeaderURL)

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := NewAuthenticatedClient(ctx, cfg, factory, blueLeaderURL)
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
		logger.V(1).Info(
			"Raft server in config",
			"node_id", server.NodeID,
			"address", server.Address,
			"voter", server.Voter,
			"leader", server.Leader,
		)
	}

	for _, i := range ReplicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := RevisionPodName(cfg.ClusterName, cfg.GreenRevision, i)

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
		alreadyVoter, err := promoteRaftPeerAndVerify(ctx, client, greenPodName)
		if err != nil {
			return fmt.Errorf("failed to promote Green pod %q to voter: %w", greenPodName, err)
		}
		if alreadyVoter {
			logger.V(1).Info("Green pod is already a voter, skipping", "pod_name", greenPodName)
			continue
		}
	}

	logger.Info("Green pods promoted to voters")
	return nil
}

// RunBlueGreenDemoteBlueNonVotersStepDown transfers leadership to Green and
// demotes Blue peers.
func RunBlueGreenDemoteBlueNonVotersStepDown(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	leaderURL, err := FindInitialLeader(ctx, logger, cfg)
	if err != nil {
		return err
	}

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := EnsureGreenLeaderBySteppingDownBlue(ctx, logger, cfg, factory, leaderURL)
	if err != nil {
		return err
	}

	if err := DemoteAllBluePods(ctx, logger, cfg, client); err != nil {
		return err
	}

	logger.Info("Blue pods demoted to non-voters")
	return nil
}

// RunBlueGreenRemoveBluePeers removes Blue peers after cutover.
func RunBlueGreenRemoveBluePeers(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	return RunBlueGreenRemovePeers(ctx, logger, cfg, cfg.BlueRevision, cfg.GreenRevision, cfg.BlueRevision, "Blue")
}

// RunBlueGreenRemoveGreenPeers removes Green peers during rollback cleanup.
func RunBlueGreenRemoveGreenPeers(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	return RunBlueGreenRemovePeers(ctx, logger, cfg, cfg.GreenRevision, cfg.BlueRevision, cfg.GreenRevision, "Green")
}

// RunBlueGreenRemovePeers removes raft peers for the specified revision using a
// preferred and fallback leader order.
func RunBlueGreenRemovePeers(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	revisionToRemove string,
	preferredLeaderRevision string,
	fallbackLeaderRevision string,
	peerColor string,
) error {
	leaderURL, err := FindPreferredLeaderForRevisions(
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

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	client, err := NewAuthenticatedClient(ctx, cfg, factory, leaderURL)
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration: %w", err)
	}

	for _, server := range config.Config.Servers {
		if !RaftServerMatchesRevision(server.NodeID, server.Address, cfg.ClusterName, revisionToRemove, cfg.ClusterReplicas) {
			continue
		}

		logger.Info(
			"Removing Raft peer",
			"target", peerColor,
			"node_id", server.NodeID,
			"address", server.Address,
		)
		if err := client.RemoveRaftPeer(ctx, server.NodeID); err != nil {
			return fmt.Errorf("failed to remove Raft peer %q: %w", server.NodeID, err)
		}
	}

	logger.Info("Raft peers removed", "target", peerColor)
	return nil
}
