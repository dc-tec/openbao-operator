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

	for i := int32(0); i < cfg.ClusterReplicas; i++ {
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
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			greenPodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.GreenRevision, i)
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
	for i := int32(0); i < cfg.ClusterReplicas; i++ {
		greenPodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.GreenRevision, i)
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
	leaderURL, err := findLeader(ctx, cfg, cfg.BlueRevision)
	if err != nil {
		logger.Info("Failed to find leader among Blue pods for consensus repair, falling back to Green", "error", err)
		leaderURL, err = findLeader(ctx, cfg, cfg.GreenRevision)
		if err != nil {
			return fmt.Errorf("failed to find leader for consensus repair: %w", err)
		}
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
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			podName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
			if nodeID == podName || strings.Contains(address, podName) {
				return true
			}
		}
		return false
	}

	isGreenServer := func(nodeID, address string) bool {
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			podName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.GreenRevision, i)
			if nodeID == podName || strings.Contains(address, podName) {
				return true
			}
		}
		return false
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
			logger.Info("Failed to demote Green peer (may already be non-voter)", "node_id", server.NodeID, "error", err)
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
	for i := int32(0); i < cfg.ClusterReplicas; i++ {
		greenPodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.GreenRevision, i)

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
	leaderURL, err := findLeader(ctx, cfg, cfg.GreenRevision)
	if err != nil {
		logger.Info("Failed to find leader among Green pods, checking Blue pods", "error", err)
		leaderURL, err = findLeader(ctx, cfg, cfg.BlueRevision)
		if err != nil {
			return "", fmt.Errorf("failed to find initial leader: %w", err)
		}
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
	for attempt := 0; attempt < maxRetries; attempt++ {
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

func demoteBlueVotersExceptLeader(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	client *openbao.Client,
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
			// Log but continue - step-down is the main action
			logger.Error(err, "Failed to demote Blue peer", "node_id", server.NodeID, "cluster_replicas", cfg.ClusterReplicas)
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
	logger.Info("Waiting for new leader election...")

	newLeaderURL, err := waitForLeaderElection(ctx, cfg, previousLeaderURL)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return "", fmt.Errorf("failed while waiting for new leader election: %w", err)
	}

	logger.Info("Finding new leader...")
	if strings.TrimSpace(newLeaderURL) != "" {
		logger.Info("New leader found", "leader_url", newLeaderURL)
		return newLeaderURL, nil
	}

	leaderURL, findErr := findLeader(ctx, cfg, cfg.GreenRevision)
	if findErr != nil {
		logger.Info("Failed to find leader among Green pods, checking Blue pods", "error", findErr)
		leaderURL, findErr = findLeader(ctx, cfg, cfg.BlueRevision)
		if findErr != nil {
			return "", fmt.Errorf("failed to find new leader after step-down (checked Green and Blue): %w", findErr)
		}
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
	return newLeaderURL, err
}

func demoteAllBluePods(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, client *openbao.Client) error {
	if client == nil {
		return fmt.Errorf("client is required to demote Blue pods")
	}

	for i := int32(0); i < cfg.ClusterReplicas; i++ {
		bluePodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
		logger.V(1).Info("Demoting Blue pod to non-voter", "pod_name", bluePodName)
		if err := client.DemoteRaftPeer(ctx, bluePodName); err != nil {
			// It might already be demoted or is non-voter, benign error usually
			logger.Info("Failed to demote Blue pod (might already be non-voter)", "pod_name", bluePodName, "error", err)
		}
	}
	return nil
}

func runBlueGreenRemoveBluePeers(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) error {
	// At this point, the leader should be a Green pod. Try Green first, then Blue.
	leaderURL, err := findLeader(ctx, cfg, cfg.GreenRevision)
	if err != nil {
		// Fall back to checking Blue pods in case leadership hasn't transferred yet
		leaderURL, err = findLeader(ctx, cfg, cfg.BlueRevision)
		if err != nil {
			return fmt.Errorf("failed to find leader: %w", err)
		}
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

	config, err := client.ReadRaftConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("failed to read Raft configuration: %w", err)
	}

	for _, server := range config.Config.Servers {
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			bluePodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
			if server.NodeID == bluePodName || strings.Contains(server.Address, bluePodName) {
				logger.Info("Removing Blue Raft peer", "node_id", server.NodeID, "address", server.Address)
				if err := client.RemoveRaftPeer(ctx, server.NodeID); err != nil {
					return fmt.Errorf("failed to remove Raft peer %q: %w", server.NodeID, err)
				}
			}
		}
	}

	logger.Info("Blue Raft peers removed")
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

	for attempt := 0; attempt < 10; attempt++ {
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
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
			return "", fmt.Errorf("context cancelled while finding leader: %w", ctx.Err())
		case <-timer.C:
		}
	}

	return "", fmt.Errorf("no leader found among %d pods", cfg.ClusterReplicas)
}

func findLeaderOnce(ctx context.Context, cfg *ExecutorConfig, revision string) (string, bool) {
	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return "", false
	}
	defer cleanup()

	for i := int32(0); i < cfg.ClusterReplicas; i++ {
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
	podName := fmt.Sprintf("%s-%d", cfg.ClusterName, ordinal)
	if revision != "" {
		podName = fmt.Sprintf("%s-%s-%d", cfg.ClusterName, revision, ordinal)
	}
	host := fmt.Sprintf("%s.%s.%s.svc", podName, cfg.ClusterName, cfg.ClusterNamespace)
	return fmt.Sprintf("https://%s:%d", host, constants.PortAPI)
}

func isBenignJoinError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already joined")
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
