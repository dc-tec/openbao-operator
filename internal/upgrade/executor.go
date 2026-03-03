package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/dc-tec/openbao-operator/internal/constants"
	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
)

const (
	leaderElectionWaitDuration = 5 * time.Second
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
			if isBenignJoinError(err) {
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

		allSynced := true
		maxDelta := uint64(0)
		missingGreen := 0
		unhealthyGreen := 0
		for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
			greenPodName := revisionPodName(cfg.ClusterName, cfg.GreenRevision, i)
			found := false
			for _, server := range state.Servers {
				if raftAutopilotServerMatchesPod(server, greenPodName) {
					found = true

					// Note: Non-voters may report as unhealthy during sync since they haven't
					// fully replicated yet. We log this for visibility but only block on delta.
					if !server.Healthy {
						unhealthyGreen++
						logger.V(1).Info("Green pod is not healthy in autopilot (expected for non-voters syncing)", "pod_name", greenPodName, "healthy", server.Healthy, "status", server.Status, "last_index", server.LastIndex)
					}

					var delta uint64
					if targetIndex > server.LastIndex {
						delta = targetIndex - server.LastIndex
					}
					if delta > maxDelta {
						maxDelta = delta
					}
					if delta > cfg.SyncThreshold {
						allSynced = false
					}
					break
				}
			}
			if !found {
				allSynced = false
				missingGreen++
				logger.V(1).Info("Green pod not found in autopilot state", "expected_pod_name", greenPodName)
			}
		}

		if allSynced {
			logger.Info("Green pods are synced", "target_index", targetIndex, "sync_threshold", cfg.SyncThreshold)
			return nil
		}

		if time.Now().After(nextProgressLog) {
			// Log all servers in autopilot state for debugging
			serverNames := make([]string, 0, len(state.Servers))
			for key, server := range state.Servers {
				serverNames = append(serverNames, fmt.Sprintf("%s(id=%s,name=%s,addr=%s)", key, server.ID, server.Name, server.Address))
			}
			logger.Info("Waiting for Green sync",
				"target_index", targetIndex,
				"max_delta", maxDelta,
				"sync_threshold", cfg.SyncThreshold,
				"missing_green", missingGreen,
				"unhealthy_green", unhealthyGreen,
				"autopilot_servers", serverNames,
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

func raftAutopilotLeaderLastIndex(state *openbao.RaftAutopilotStateResponse) (uint64, bool) {
	if state == nil {
		return 0, false
	}

	if state.Leader != "" {
		if server, ok := state.Servers[state.Leader]; ok {
			return server.LastIndex, true
		}

		for _, server := range state.Servers {
			if server.ID == state.Leader || server.Name == state.Leader || server.Status == "leader" {
				return server.LastIndex, true
			}
		}
	}

	for _, server := range state.Servers {
		if server.Status == "leader" {
			return server.LastIndex, true
		}
	}

	return 0, false
}

func raftAutopilotMaxLastIndex(state *openbao.RaftAutopilotStateResponse) uint64 {
	if state == nil {
		return 0
	}

	var max uint64
	for _, server := range state.Servers {
		if server.LastIndex > max {
			max = server.LastIndex
		}
	}

	return max
}

func raftAutopilotServerMatchesPod(server openbao.RaftAutopilotServerState, podName string) bool {
	if podName == "" {
		return false
	}

	if server.ID == podName || server.Name == podName {
		return true
	}

	return strings.Contains(server.Address, podName)
}

func countMissingGreenServers(cfg *ExecutorConfig, config *openbao.RaftConfigurationResponse) int {
	if cfg == nil || config == nil {
		return 0
	}

	missing := 0
	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := revisionPodName(cfg.ClusterName, cfg.GreenRevision, i)
		found := false
		for _, server := range config.Config.Servers {
			if server.NodeID == greenPodName || strings.Contains(server.Address, greenPodName) {
				found = true
				break
			}
		}
		if !found {
			missing++
		}
	}

	return missing
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
	leaderURL, err := findLeaderWithFallback(ctx, logger, cfg, cfg.BlueRevision, cfg.GreenRevision, "Blue", "Green")
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

	// Helper to classify a Raft server as Blue or Green based on pod naming.
	isBlueServer := func(nodeID, address string) bool {
		return raftServerMatchesRevision(nodeID, address, cfg.ClusterName, cfg.BlueRevision, cfg.ClusterReplicas)
	}

	isGreenServer := func(nodeID, address string) bool {
		return raftServerMatchesRevision(nodeID, address, cfg.ClusterName, cfg.GreenRevision, cfg.ClusterReplicas)
	}

	// First pass: ensure all Blue servers are voters.
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

	// Second pass: ensure all Green servers are non-voters.
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

	// Read current Raft configuration to check voter status
	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration: %w", err)
	}

	// Build a map of server ID -> voter status for quick lookup
	voterStatus := make(map[string]bool)
	for _, server := range config.Config.Servers {
		voterStatus[server.NodeID] = server.Voter
		logger.V(1).Info("Raft server in config",
			"node_id", server.NodeID,
			"address", server.Address,
			"voter", server.Voter,
			"leader", server.Leader)
	}

	// Promote each Green pod from non-voter to voter individually
	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		greenPodName := revisionPodName(cfg.ClusterName, cfg.GreenRevision, i)

		// Check if already a voter (autopilot may have auto-promoted)
		if isVoter, found := voterStatus[greenPodName]; found {
			if isVoter {
				logger.V(1).Info("Green pod is already a voter, skipping", "pod_name", greenPodName)
				continue
			}
			logger.V(1).Info("Green pod found as non-voter, will promote", "pod_name", greenPodName)
		} else {
			logger.Info("WARNING: Green pod not found in Raft config", "pod_name", greenPodName)
			// Still try to promote - it might be in the cluster with a different ID
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

func findInitialLeader(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) (string, error) {
	leaderURL, err := findLeaderWithFallback(ctx, logger, cfg, cfg.GreenRevision, cfg.BlueRevision, "Green", "Blue")
	if err != nil {
		return "", fmt.Errorf("failed to find initial leader: %w", err)
	}
	logger.Info("Initial leader found", "leader_url", leaderURL)
	return leaderURL, nil
}

func ensureGreenLeaderBySteppingDownBlue(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	factory *openbao.ClientFactory,
	leaderURL string,
) (*openbao.Client, error) {
	const maxRetries = 10
	bluePrefix := fmt.Sprintf("%s-%s-", cfg.ClusterName, cfg.BlueRevision)

	var client *openbao.Client
	for _, attempt := range attemptOrdinals(maxRetries) {
		var err error
		client, err = clientForLeaderURL(ctx, cfg, factory, leaderURL)
		if err != nil {
			return nil, err
		}

		config, err := client.ReadRaftConfiguration(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read Raft configuration: %w", err)
		}

		leaderID, leaderIsBlue := raftLeaderInfo(config, bluePrefix)
		if !leaderIsBlue {
			logger.Info("Leader is not Blue (assumed Green), proceeding to demotion")
			return client, nil
		}

		logger.Info("Current leader is Blue", "leader_id", leaderID, "attempt", attempt+1, "max_retries", maxRetries)
		demoteBlueVotersExceptLeader(ctx, logger, cfg, client, config, leaderID, bluePrefix)
		stepDownLeader(ctx, logger, client)

		newLeaderURL, err := waitForNewLeaderURL(ctx, logger, cfg, leaderURL)
		if err != nil {
			return nil, err
		}
		leaderURL = newLeaderURL
	}

	return nil, fmt.Errorf("failed to transfer leadership to Green node after %d attempts", maxRetries)
}

func clientForLeaderURL(ctx context.Context, cfg *ExecutorConfig, factory *openbao.ClientFactory, leaderURL string) (*openbao.Client, error) {
	token, err := loginJWT(ctx, cfg, leaderURL)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	client, err := factory.NewWithToken(leaderURL, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}
	return client, nil
}

func raftLeaderInfo(config *openbao.RaftConfigurationResponse, bluePrefix string) (string, bool) {
	if config == nil {
		return "", false
	}

	for _, server := range config.Config.Servers {
		if !server.Leader {
			continue
		}
		leaderID := server.NodeID
		return leaderID, isBlueRaftServer(server.NodeID, server.Address, bluePrefix)
	}

	return "", false
}

func isBlueRaftServer(nodeID string, address string, bluePrefix string) bool {
	return strings.HasPrefix(nodeID, bluePrefix) || strings.Contains(address, bluePrefix)
}

type raftPeerDemoter interface {
	DemoteRaftPeer(ctx context.Context, serverID string) error
}

func demoteBlueVotersExceptLeader(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	client raftPeerDemoter,
	config *openbao.RaftConfigurationResponse,
	leaderID string,
	bluePrefix string,
) {
	if client == nil || config == nil {
		return
	}

	for _, server := range config.Config.Servers {
		if !server.Voter || server.NodeID == leaderID {
			continue
		}
		if !isBlueRaftServer(server.NodeID, server.Address, bluePrefix) {
			continue
		}

		logger.Info("Demoting Blue peer before step-down to bias election", "node_id", server.NodeID)
		if err := client.DemoteRaftPeer(ctx, server.NodeID); err != nil {
			if isBenignDemoteError(err) {
				logger.V(1).Info("Blue peer already non-voter before step-down", "node_id", server.NodeID)
				continue
			}
			// Log but continue - step-down is the main action.
			logger.Error(err, "Failed to demote Blue peer before step-down", "node_id", server.NodeID, "cluster_replicas", cfg.ClusterReplicas)
		}
	}
}

func stepDownLeader(ctx context.Context, logger logr.Logger, client *openbao.Client) {
	logger.Info("Stepping down Blue leader to transfer leadership to Green")
	if err := client.StepDown(ctx); err != nil {
		// If step down fails, maybe we lost connection? Just log and retry loop.
		logger.Error(err, "Failed to step down leader")
	}
}

func waitForNewLeaderURL(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, previousLeaderURL string) (string, error) {
	return waitForNewLeaderURLWithFuncs(ctx, logger, cfg, previousLeaderURL, waitForLeaderElection, findLeaderWithFallback)
}

func waitForNewLeaderURLWithFuncs(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	previousLeaderURL string,
	waitFn func(context.Context, *ExecutorConfig, string) (string, error),
	fallbackFn func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error),
) (string, error) {
	logger.Info("Waiting for new leader election...")

	newLeaderURL, err := waitFn(ctx, cfg, previousLeaderURL)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return "", newExecutorReasonedError(reasonElectionTimeout, "failed while waiting for new leader election", err)
	}

	logger.Info("Finding new leader...")
	if strings.TrimSpace(newLeaderURL) != "" {
		logger.Info("New leader found", "leader_url", newLeaderURL)
		return newLeaderURL, nil
	}

	leaderURL, findErr := fallbackFn(ctx, logger, cfg, cfg.GreenRevision, cfg.BlueRevision, "Green", "Blue")
	if findErr != nil {
		reasonCode := reasonCodeFromError(findErr)
		if reasonCode == "" {
			reasonCode = reasonFallbackLeaderNotFound
		}
		return "", newExecutorReasonedError(reasonCode, "failed to find new leader after step-down", findErr)
	}
	logger.Info("New leader found", "leader_url", leaderURL)
	return leaderURL, nil
}

func waitForLeaderElection(ctx context.Context, cfg *ExecutorConfig, previousLeaderURL string) (string, error) {
	var newLeaderURL string
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, leaderElectionWaitDuration, true, func(ctx context.Context) (bool, error) {
		if url, ok := findLeaderOnce(ctx, cfg, cfg.GreenRevision); ok {
			newLeaderURL = url
			return true, nil
		}
		if url, ok := findLeaderOnce(ctx, cfg, cfg.BlueRevision); ok {
			// Only consider it "new" if leadership moved away from the pre-stepdown leader.
			if url != previousLeaderURL {
				newLeaderURL = url
				return true, nil
			}
			// Keep the last observed leader as a fallback; the outer loop will step down again if needed.
			newLeaderURL = url
		}
		return false, nil
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.Canceled) {
			err = newExecutorReasonedError(reasonElectionTimeout, "leader election did not converge within wait duration", err)
		} else if reasonCode := reasonCodeFromContextError(err); reasonCode != "" {
			err = newExecutorReasonedError(reasonCode, "leader election was interrupted", err)
		}
	}
	return newLeaderURL, err
}

func demoteAllBluePods(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, client raftPeerDemoter) error {
	if client == nil {
		return fmt.Errorf("client is required to demote Blue pods")
	}

	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		bluePodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
		logger.V(1).Info("Demoting Blue pod to non-voter", "pod_name", bluePodName)
		if err := client.DemoteRaftPeer(ctx, bluePodName); err != nil {
			if isBenignDemoteError(err) {
				logger.V(1).Info("Blue pod already non-voter after leader transfer", "pod_name", bluePodName)
				continue
			}
			return fmt.Errorf("failed to demote Blue pod %q to non-voter: %w", bluePodName, err)
		}
	}
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

	leaderURL, err := findLeaderWithFallback(
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

func loginJWT(ctx context.Context, cfg *ExecutorConfig, baseURL string) (string, error) {
	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()

	return factory.LoginJWT(ctx, baseURL, cfg.JWTAuthRole, cfg.JWTToken)
}

func findLeader(ctx context.Context, cfg *ExecutorConfig, revision string) (string, error) {
	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()

	for range attemptOrdinals(10) {
		for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
			url := podURL(cfg, revision, i)
			client, err := factory.New(url)
			if err != nil {
				continue
			}
			isLeader, err := client.IsLeader(ctx)
			if err != nil {
				continue
			}
			if isLeader {
				return url, nil
			}
		}

		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			reasonCode := reasonCodeFromContextError(ctx.Err())
			if reasonCode == "" {
				reasonCode = reasonContextCanceled
			}
			return "", newExecutorReasonedError(reasonCode, "context cancelled while finding leader", ctx.Err())
		case <-timer.C:
		}
	}

	return "", newExecutorReasonedError(reasonPrimaryLeaderNotFound, fmt.Sprintf("no leader found among %d pods", cfg.ClusterReplicas), nil)
}

func findLeaderWithFallback(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	primaryRevision string,
	fallbackRevision string,
	primaryLabel string,
	fallbackLabel string,
) (string, error) {
	return findLeaderWithFallbackUsing(ctx, logger, cfg, primaryRevision, fallbackRevision, primaryLabel, fallbackLabel, findLeader)
}

type leaderFinder func(context.Context, *ExecutorConfig, string) (string, error)

func findLeaderWithFallbackUsing(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	primaryRevision string,
	fallbackRevision string,
	primaryLabel string,
	fallbackLabel string,
	finder leaderFinder,
) (string, error) {
	policy := newLeaderSearchPolicy(primaryRevision, fallbackRevision, primaryLabel, fallbackLabel)
	outcome := resolveLeaderWithPolicyUsing(ctx, cfg, policy, finder)
	if outcome.Value != "" {
		return outcome.Value, nil
	}

	if policy.AllowFallback {
		logger.Info(
			fmt.Sprintf("Failed to find leader among %s pods, checking %s pods", primaryLabel, fallbackLabel),
			"error", outcome.PrimaryError,
			"decision_path", outcome.DecisionPath,
			"reason_code", outcome.ReasonCode,
		)
		return "", newExecutorReasonedError(
			outcome.ReasonCode,
			fmt.Sprintf("failed to find leader (checked %s and %s)", primaryLabel, fallbackLabel),
			outcome.FallbackError,
		)
	}

	return "", newExecutorReasonedError(
		outcome.ReasonCode,
		fmt.Sprintf("failed to find leader among %s pods", primaryLabel),
		outcome.PrimaryError,
	)
}

func findLeaderOnce(ctx context.Context, cfg *ExecutorConfig, revision string) (string, bool) {
	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return "", false
	}
	defer cleanup()

	for _, i := range replicaOrdinals(cfg.ClusterReplicas) {
		url := podURL(cfg, revision, i)
		client, err := factory.New(url)
		if err != nil {
			continue
		}
		isLeader, err := client.IsLeader(ctx)
		if err != nil {
			continue
		}
		if isLeader {
			return url, true
		}
	}

	return "", false
}

func podURL(cfg *ExecutorConfig, revision string, ordinal int32) string {
	podName := revisionPodName(cfg.ClusterName, revision, ordinal)
	host := fmt.Sprintf("%s.%s.%s.svc", podName, cfg.ClusterName, cfg.ClusterNamespace)
	return fmt.Sprintf("https://%s:%d", host, constants.PortAPI)
}

func revisionPodName(clusterName string, revision string, ordinal int32) string {
	if revision == "" {
		return fmt.Sprintf("%s-%d", clusterName, ordinal)
	}
	return fmt.Sprintf("%s-%s-%d", clusterName, revision, ordinal)
}

func raftServerMatchesRevision(nodeID string, address string, clusterName string, revision string, replicas int32) bool {
	for _, i := range replicaOrdinals(replicas) {
		podName := revisionPodName(clusterName, revision, i)
		if nodeID == podName || strings.Contains(address, podName) {
			return true
		}
	}
	return false
}

func replicaOrdinals(replicas int32) []int32 {
	if replicas <= 0 {
		return nil
	}
	ordinals := make([]int32, 0, replicas)
	for i := int32(0); i < replicas; i++ {
		ordinals = append(ordinals, i)
	}
	return ordinals
}

func attemptOrdinals(maxAttempts int) []int {
	if maxAttempts <= 0 {
		return nil
	}
	ordinals := make([]int, 0, maxAttempts)
	for i := 0; i < maxAttempts; i++ {
		ordinals = append(ordinals, i)
	}
	return ordinals
}

func isBenignJoinError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already joined")
}

func isBenignDemoteError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already a non-voter") ||
		strings.Contains(message, "already non-voter") ||
		strings.Contains(message, "already non voter")
}

func newOpenBaoClientFactory(cfg *ExecutorConfig) (*openbao.ClientFactory, func(), error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("config is required")
	}

	mgr := openbao.NewClientManager(openbao.ClientConfig{
		ClusterKey:                     fmt.Sprintf("%s/%s", cfg.ClusterNamespace, cfg.ClusterName),
		CACert:                         cfg.TLSCACert,
		RateLimitQPS:                   cfg.ClientQPS,
		RateLimitBurst:                 cfg.ClientBurst,
		CircuitBreakerFailureThreshold: cfg.ClientCircuitBreakerFailureThreshold,
		CircuitBreakerOpenDuration:     cfg.ClientCircuitBreakerOpenDuration,
	})

	factory := mgr.FactoryFor(fmt.Sprintf("%s/%s", cfg.ClusterNamespace, cfg.ClusterName), cfg.TLSCACert)
	return factory, mgr.Close, nil
}
