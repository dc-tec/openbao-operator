package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/openbao/operator/internal/constants"
	openbao "github.com/openbao/operator/internal/openbao"
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

	client, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: leaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
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

	for i := int32(0); i < cfg.ClusterReplicas; i++ {
		greenPodURL := podURL(cfg, cfg.GreenRevision, i)
		logger.V(1).Info("Joining Green pod as non-voter", "green_pod_url", greenPodURL)
		client, err := openbao.NewClient(openbao.ClientConfig{
			BaseURL: greenPodURL,
			Token:   token,
			CACert:  cfg.TLSCACert,
		})
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

	client, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: blueLeaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
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

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for Green sync: %w", ctx.Err())
		case <-time.After(2 * time.Second):
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

	client, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: blueLeaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
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
	// Find initial leader (Green preferred, then Blue)
	leaderURL, err := findLeader(ctx, cfg, cfg.GreenRevision)
	if err != nil {
		logger.Info("Failed to find leader among Green pods, checking Blue pods", "error", err)
		leaderURL, err = findLeader(ctx, cfg, cfg.BlueRevision)
		if err != nil {
			return fmt.Errorf("failed to find initial leader: %w", err)
		}
	}
	logger.Info("Initial leader found", "leader_url", leaderURL)

	var client *openbao.Client
	maxRetries := 10
	greenLeaderElected := false

	// Loop to ensure we have a Green leader
	for attempt := 0; attempt < maxRetries; attempt++ {
		token, err := loginJWT(ctx, cfg, leaderURL)
		if err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}

		client, err = openbao.NewClient(openbao.ClientConfig{
			BaseURL: leaderURL,
			Token:   token,
			CACert:  cfg.TLSCACert,
		})
		if err != nil {
			return fmt.Errorf("failed to create OpenBao client: %w", err)
		}

		config, err := client.ReadRaftConfiguration(ctx)
		if err != nil {
			return fmt.Errorf("failed to read Raft configuration: %w", err)
		}

		// Check if the current leader is from the Blue revision.
		currentLeaderBlue := false
		var currentLeaderID string

		for _, server := range config.Config.Servers {
			if server.Leader {
				for i := int32(0); i < cfg.ClusterReplicas; i++ {
					bluePodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
					if server.NodeID == bluePodName || strings.Contains(server.Address, bluePodName) {
						currentLeaderBlue = true
						currentLeaderID = server.NodeID
						break
					}
				}
			}
		}

		if !currentLeaderBlue {
			logger.Info("Leader is not Blue (assumed Green), proceeding to demotion")
			greenLeaderElected = true
			break
		}

		logger.Info("Current leader is Blue", "leader_id", currentLeaderID, "attempt", attempt+1, "max_retries", maxRetries)

		// Optimization: Demote other Blue voters to ensure Green wins the election after step-down
		for _, server := range config.Config.Servers {
			if server.Voter && server.NodeID != currentLeaderID {
				isBluePeer := false
				for i := int32(0); i < cfg.ClusterReplicas; i++ {
					bluePodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
					if server.NodeID == bluePodName || strings.Contains(server.Address, bluePodName) {
						isBluePeer = true
						break
					}
				}

				if isBluePeer {
					logger.Info("Demoting Blue peer before step-down to bias election", "node_id", server.NodeID)
					if err := client.DemoteRaftPeer(ctx, server.NodeID); err != nil {
						// Log but continue - step-down is the main action
						logger.Error(err, "Failed to demote Blue peer")
					}
				}
			}
		}

		logger.Info("Stepping down Blue leader to transfer leadership to Green")
		if err := client.StepDown(ctx); err != nil {
			// If step down fails, maybe we lost connection? Just log and retry loop.
			logger.Error(err, "Failed to step down leader")
		}

		// Wait for new leader to be elected
		logger.Info("Waiting for new leader election...")
		time.Sleep(5 * time.Second)

		// Re-find the new leader (should now be a Green node, but might be Blue)
		logger.Info("Finding new leader...")
		leaderURL, err = findLeader(ctx, cfg, cfg.GreenRevision)
		if err != nil {
			logger.Info("Failed to find leader among Green pods, checking Blue pods", "error", err)
			leaderURL, err = findLeader(ctx, cfg, cfg.BlueRevision)
			if err != nil {
				return fmt.Errorf("failed to find new leader after step-down (checked Green and Blue): %w", err)
			}
		}
		logger.Info("New leader found", "leader_url", leaderURL)
	}

	if !greenLeaderElected {
		return fmt.Errorf("failed to transfer leadership to Green node after %d attempts", maxRetries)
	}

	// Demote remaining Blue pods (e.g. the former leader) from voter to non-voter
	for i := int32(0); i < cfg.ClusterReplicas; i++ {
		bluePodName := fmt.Sprintf("%s-%s-%d", cfg.ClusterName, cfg.BlueRevision, i)
		logger.V(1).Info("Demoting Blue pod to non-voter", "pod_name", bluePodName)
		if err := client.DemoteRaftPeer(ctx, bluePodName); err != nil {
			// It might already be demoted or is non-voter, benign error usually
			logger.Info("Failed to demote Blue pod (might already be non-voter)", "pod_name", bluePodName, "error", err)
		}
	}

	logger.Info("Blue pods demoted to non-voters")
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

	client, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: leaderURL,
		Token:   token,
		CACert:  cfg.TLSCACert,
	})
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
	client, err := openbao.NewClient(openbao.ClientConfig{
		BaseURL: baseURL,
		CACert:  cfg.TLSCACert,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	token, err := client.LoginJWT(ctx, cfg.JWTAuthRole, cfg.JWTToken)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate using JWT Auth: %w", err)
	}

	return token, nil
}

func findLeader(ctx context.Context, cfg *ExecutorConfig, revision string) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		for i := int32(0); i < cfg.ClusterReplicas; i++ {
			url := podURL(cfg, revision, i)
			client, err := openbao.NewClient(openbao.ClientConfig{
				BaseURL: url,
				CACert:  cfg.TLSCACert,
			})
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

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context cancelled while finding leader: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}

	return "", fmt.Errorf("no leader found among %d pods", cfg.ClusterReplicas)
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
	msg := err.Error()
	if strings.Contains(msg, "already joined") {
		return true
	}
	return false
}
