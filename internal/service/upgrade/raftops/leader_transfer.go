package raftops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	defaultLeaderTransferMaxRetries = 10
	leaderElectionWaitDuration      = 5 * time.Second
)

// LeaderTransferClient is the subset of OpenBao client functionality required
// for leader transfer and voter management.
type LeaderTransferClient interface {
	ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error)
	DemoteRaftPeer(context.Context, string) error
	StepDown(context.Context) error
}

// LeaderTransferClientResolver resolves a leader-scoped client.
type LeaderTransferClientResolver func(context.Context, string) (LeaderTransferClient, error)

// LeaderTransferWaitFunc waits for a new leader URL to become available.
type LeaderTransferWaitFunc func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error)

// RaftPeerDemoter demotes raft peers to non-voters.
type RaftPeerDemoter interface {
	DemoteRaftPeer(ctx context.Context, serverID string) error
}

// LeaderElectionWaitFunc waits for leader election and returns the outcome.
type LeaderElectionWaitFunc func(context.Context, *ExecutorConfig, string) LeaderElectionOutcome

// LeaderFallbackResolver resolves a leader after the election wait path times out.
type LeaderFallbackResolver func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error)

// LeaderOnceFinder performs a single leader check without retries.
type LeaderOnceFinder func(context.Context, *ExecutorConfig, string) (string, bool)

const (
	leaderTransferStateResolveCurrentLeader = "ResolveCurrentLeader"
	leaderTransferStateInspectRaftConfig    = "InspectRaftConfig"
	leaderTransferStateBiasElection         = "BiasElection"
	leaderTransferStateStepDown             = "StepDown"
	leaderTransferStateAwaitNewLeader       = "AwaitNewLeader"
	leaderTransferStateValidateGreenLeader  = "ValidateGreenLeader"
)

// FindInitialLeader prefers a Green leader and falls back to Blue.
func FindInitialLeader(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) (string, error) {
	leaderURL, err := FindPreferredLeaderForRevisions(
		ctx,
		logger,
		cfg,
		cfg.GreenRevision,
		cfg.BlueRevision,
		"Green",
		"Blue",
	)
	if err != nil {
		return "", fmt.Errorf("failed to find initial leader: %w", err)
	}
	logger.Info("Initial leader found", "leader_url", leaderURL)
	return leaderURL, nil
}

// EnsureGreenLeaderBySteppingDownBlue transfers leadership away from a Blue
// leader and waits for a Green leader to take over.
func EnsureGreenLeaderBySteppingDownBlue(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	factory *openbao.ClientFactory,
	leaderURL string,
) (LeaderTransferClient, error) {
	resolver := func(ctx context.Context, leaderURL string) (LeaderTransferClient, error) {
		return clientForLeaderURL(ctx, cfg, factory, leaderURL)
	}
	return EnsureGreenLeaderBySteppingDownBlueWithFuncs(
		ctx,
		logger,
		cfg,
		leaderURL,
		RetryPolicy{MaxAttempts: defaultLeaderTransferMaxRetries},
		resolver,
		WaitForNewLeaderURL,
	)
}

// EnsureGreenLeaderBySteppingDownBlueWithFuncs is the injectable form used by
// tests.
func EnsureGreenLeaderBySteppingDownBlueWithFuncs(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	leaderURL string,
	policy RetryPolicy,
	resolveClient LeaderTransferClientResolver,
	waitForLeader LeaderTransferWaitFunc,
) (LeaderTransferClient, error) {
	policy = NormalizeLeaderTransferRetryPolicy(policy)
	bluePrefix := fmt.Sprintf("%s-%s-", cfg.ClusterName, cfg.BlueRevision)

	for _, attempt := range AttemptOrdinals(policy.MaxAttempts) {
		attemptNumber := attempt + 1
		state := leaderTransferStateResolveCurrentLeader
		var client LeaderTransferClient
		var config *portopenbao.RaftConfigurationResponse
		var leaderID string
		var leaderIsBlue bool

		for {
			switch state {
			case leaderTransferStateResolveCurrentLeader:
				resolvedClient, err := resolveClient(ctx, leaderURL)
				if err != nil {
					return nil, NewExecutorReasonedError(ReasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				client = resolvedClient
				state = leaderTransferStateInspectRaftConfig

			case leaderTransferStateInspectRaftConfig:
				currentConfig, err := client.ReadRaftConfiguration(ctx)
				if err != nil {
					return nil, NewExecutorReasonedError(ReasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				config = currentConfig
				leaderID, leaderIsBlue = RaftLeaderInfoForRevision(config, cfg)
				if leaderID == "" {
					return nil, NewExecutorReasonedError(ReasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), errors.New("raft leader not found in configuration"))
				}
				if !leaderIsBlue {
					logger.Info("Leader is not Blue (assumed Green), proceeding to demotion", "state", leaderTransferStateValidateGreenLeader, "attempt", attemptNumber, "max_retries", policy.MaxAttempts)
					return client, nil
				}

				logger.Info("Current leader is Blue", "leader_id", leaderID, "state", state, "attempt", attemptNumber, "max_retries", policy.MaxAttempts)
				state = leaderTransferStateBiasElection

			case leaderTransferStateBiasElection:
				if err := DemoteBlueVotersExceptLeader(ctx, logger, cfg, client, config, leaderID, bluePrefix); err != nil {
					return nil, NewExecutorReasonedError(ReasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				state = leaderTransferStateStepDown

			case leaderTransferStateStepDown:
				classification, err := StepDownLeader(ctx, logger, client)
				if err != nil && classification == BenignErrorClassificationFatal {
					return nil, NewExecutorReasonedError(ReasonStepDownFatal, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				state = leaderTransferStateAwaitNewLeader

			case leaderTransferStateAwaitNewLeader:
				newLeaderURL, err := waitForLeader(ctx, logger, cfg, leaderURL)
				if err != nil {
					return nil, NewExecutorReasonedError(ReasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				leaderURL = newLeaderURL
				state = leaderTransferStateValidateGreenLeader

			case leaderTransferStateValidateGreenLeader:
				logger.V(1).Info(
					"Leader transfer attempt completed; validating leader on next attempt",
					"state", state,
					"attempt", attemptNumber,
					"max_retries", policy.MaxAttempts,
					"next_leader_url", leaderURL,
				)
				goto nextAttempt
			}
		}

	nextAttempt:
	}

	return nil, NewExecutorReasonedError(
		ReasonLeaderTransferRetriesExhausted,
		fmt.Sprintf("failed to transfer leadership to Green node after %d attempts", policy.MaxAttempts),
		nil,
	)
}

// NormalizeLeaderTransferRetryPolicy normalizes defaults for leader transfer.
func NormalizeLeaderTransferRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaultLeaderTransferMaxRetries
	}
	return policy
}

func clientForLeaderURL(ctx context.Context, cfg *ExecutorConfig, factory *openbao.ClientFactory, leaderURL string) (*openbao.Client, error) {
	client, err := NewAuthenticatedClient(ctx, cfg, factory, leaderURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}
	return client, nil
}

// RaftLeaderInfoForRevision returns the leader and whether it belongs to the
// configured Blue workload. Exact expected pod names are required because the
// original unrevisioned StatefulSet prefix also prefixes revisioned Green Pods.
func RaftLeaderInfoForRevision(config *portopenbao.RaftConfigurationResponse, cfg *ExecutorConfig) (string, bool) {
	if config == nil || cfg == nil {
		return "", false
	}

	for _, server := range config.Config.Servers {
		if !server.Leader {
			continue
		}
		return server.NodeID, RaftServerMatchesRevision(
			server.NodeID,
			server.Address,
			cfg.ClusterName,
			cfg.BlueRevision,
			cfg.ClusterReplicas,
		)
	}

	return "", false
}

// IsBlueRaftServer reports whether the raft server belongs to the Blue revision.
func IsBlueRaftServer(nodeID string, address string, bluePrefix string) bool {
	return strings.HasPrefix(nodeID, bluePrefix) || strings.Contains(address, bluePrefix)
}

// DemoteBlueVotersExceptLeader demotes non-leader Blue voters to bias a Green
// election after the step-down.
func DemoteBlueVotersExceptLeader(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	client RaftPeerDemoter,
	config *portopenbao.RaftConfigurationResponse,
	leaderID string,
	bluePrefix string,
) error {
	if client == nil || config == nil {
		return nil
	}

	for _, server := range config.Config.Servers {
		if !server.Voter || server.NodeID == leaderID {
			continue
		}
		isBlue := RaftServerMatchesRevision(server.NodeID, server.Address, cfg.ClusterName, cfg.BlueRevision, cfg.ClusterReplicas)
		// Retain the prefix-based contract for older direct callers that did not
		// populate BlueRevision. Production passes "<cluster>--" for an
		// unrevisioned Blue workload and therefore uses exact expected pod names.
		if cfg.BlueRevision == "" && bluePrefix != "" && bluePrefix != cfg.ClusterName+"--" {
			isBlue = IsBlueRaftServer(server.NodeID, server.Address, bluePrefix)
		}
		if !isBlue {
			continue
		}

		logger.Info("Demoting Blue peer before step-down to bias election", "node_id", server.NodeID)
		if err := client.DemoteRaftPeer(ctx, server.NodeID); err != nil {
			classification := ClassifyDemoteError(err)
			if classification == BenignErrorClassificationBenign {
				logger.V(1).Info("Blue peer already non-voter before step-down", "node_id", server.NodeID)
				continue
			}
			if classification == BenignErrorClassificationRetryable {
				logger.Error(err, "Failed to demote Blue peer before step-down; continuing", "node_id", server.NodeID, "cluster_replicas", cfg.ClusterReplicas, "error_classification", classification)
				continue
			}

			return NewExecutorReasonedError(
				ReasonDemoteFatal,
				fmt.Sprintf("failed to demote Blue peer %q before step-down", server.NodeID),
				err,
			)
		}
	}

	return nil
}

// StepDownLeader steps down the current leader.
func StepDownLeader(ctx context.Context, logger logr.Logger, client LeaderTransferClient) (BenignErrorClassification, error) {
	logger.Info("Stepping down Blue leader to transfer leadership to Green")
	if err := client.StepDown(ctx); err != nil {
		classification := ClassifyStepDownError(err)
		logger.Error(err, "Failed to step down leader", "error_classification", classification)
		return classification, err
	}
	return BenignErrorClassificationBenign, nil
}

// WaitForNewLeaderURL waits for a new leader and falls back to leader search
// when the election wait path times out.
func WaitForNewLeaderURL(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, previousLeaderURL string) (string, error) {
	return WaitForNewLeaderURLWithFuncs(ctx, logger, cfg, previousLeaderURL, WaitForLeaderElectionOutcome, FindAnyLeader)
}

// WaitForNewLeaderURLWithFuncs is the injectable form used by tests.
func WaitForNewLeaderURLWithFuncs(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	previousLeaderURL string,
	waitFn LeaderElectionWaitFunc,
	fallbackFn LeaderFallbackResolver,
) (string, error) {
	logger.Info("Waiting for new leader election...")

	waitOutcome := waitFn(ctx, cfg, previousLeaderURL)
	if ctxErr := ctx.Err(); ctxErr != nil {
		reasonCode := ReasonCodeFromContextError(ctxErr)
		if reasonCode == "" {
			reasonCode = ReasonContextCanceled
		}
		return "", NewExecutorReasonedError(reasonCode, "failed while waiting for new leader election", ctxErr)
	}
	if waitOutcome.WaitError != nil && !errors.Is(waitOutcome.WaitError, context.DeadlineExceeded) && !errors.Is(waitOutcome.WaitError, context.Canceled) {
		reasonCode := ReasonCodeFromError(waitOutcome.WaitError)
		if reasonCode == "" {
			reasonCode = ReasonElectionTimeout
		}
		return "", NewExecutorReasonedError(reasonCode, "failed while waiting for new leader election", waitOutcome.WaitError)
	}

	logger.Info(
		"Leader election wait completed",
		"decision_path", waitOutcome.DecisionPath,
		"reason_code", waitOutcome.ReasonCode,
		"leader_url", waitOutcome.Value,
	)
	if waitOutcome.DecisionPath == DecisionPathElectionObservedNewLeader && strings.TrimSpace(waitOutcome.Value) != "" {
		logger.Info("New leader found", "leader_url", waitOutcome.Value)
		return waitOutcome.Value, nil
	}
	if waitOutcome.DecisionPath == DecisionPathElectionObservedSameLeader && strings.TrimSpace(waitOutcome.Value) != "" {
		logger.Info("Leader election retained previous leader; proceeding with observed leader", "leader_url", waitOutcome.Value)
		return waitOutcome.Value, nil
	}

	logger.Info("Finding new leader via fallback search...")
	leaderURL, findErr := fallbackFn(ctx, logger, cfg, cfg.GreenRevision, cfg.BlueRevision)
	if findErr != nil {
		reasonCode := ReasonCodeFromError(findErr)
		if reasonCode == "" {
			reasonCode = ReasonFallbackLeaderNotFound
		}
		return "", NewExecutorReasonedError(reasonCode, "failed to find new leader after step-down", findErr)
	}
	logger.Info("New leader found", "leader_url", leaderURL)
	return leaderURL, nil
}

// WaitForLeaderElectionOutcome waits for a new leader to emerge.
func WaitForLeaderElectionOutcome(ctx context.Context, cfg *ExecutorConfig, previousLeaderURL string) LeaderElectionOutcome {
	return WaitForLeaderElectionWithFinderAndPolicy(
		ctx,
		cfg,
		previousLeaderURL,
		RetryPolicy{
			AttemptInterval: 500 * time.Millisecond,
			ElectionWait:    leaderElectionWaitDuration,
		},
		FindLeaderOnce,
	)
}

// WaitForLeaderElectionWithFinderAndPolicy waits for a new leader using the
// provided finder and retry policy.
func WaitForLeaderElectionWithFinderAndPolicy(
	ctx context.Context,
	cfg *ExecutorConfig,
	previousLeaderURL string,
	policy RetryPolicy,
	finder LeaderOnceFinder,
) LeaderElectionOutcome {
	policy = NormalizeElectionRetryPolicy(policy)

	outcome := LeaderElectionOutcome{
		DecisionPath: DecisionPathElectionTimeout,
		ReasonCode:   ReasonElectionTimeout,
	}
	lastObservedLeaderURL := ""

	err := wait.PollUntilContextTimeout(ctx, policy.AttemptInterval, policy.ElectionWait, true, func(ctx context.Context) (bool, error) {
		if url, ok := finder(ctx, cfg, cfg.GreenRevision); ok {
			outcome.Value = url
			outcome.DecisionPath = DecisionPathElectionObservedNewLeader
			outcome.ReasonCode = ReasonElectionNewLeaderFound
			return true, nil
		}

		if url, ok := finder(ctx, cfg, cfg.BlueRevision); ok {
			if url != previousLeaderURL {
				outcome.Value = url
				outcome.DecisionPath = DecisionPathElectionObservedNewLeader
				outcome.ReasonCode = ReasonElectionNewLeaderFound
				return true, nil
			}

			lastObservedLeaderURL = url
		}
		return false, nil
	})
	if err == nil {
		return outcome
	}

	if errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.Canceled) {
		if strings.TrimSpace(lastObservedLeaderURL) != "" {
			outcome.Value = lastObservedLeaderURL
			outcome.DecisionPath = DecisionPathElectionObservedSameLeader
			outcome.ReasonCode = ReasonElectionSameLeaderSeen
		}
		outcome.WaitError = NewExecutorReasonedError(ReasonElectionTimeout, "leader election did not converge within wait duration", err)
		return outcome
	}

	if reasonCode := ReasonCodeFromContextError(err); reasonCode != "" {
		outcome.DecisionPath = DecisionPathFromReasonCode(reasonCode)
		outcome.ReasonCode = reasonCode
		outcome.WaitError = NewExecutorReasonedError(reasonCode, "leader election was interrupted", err)
		return outcome
	}

	outcome.WaitError = NewExecutorReasonedError(ReasonElectionTimeout, "leader election did not converge within wait duration", err)
	return outcome
}

// NormalizeElectionRetryPolicy normalizes the leader election polling policy.
func NormalizeElectionRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.AttemptInterval <= 0 {
		policy.AttemptInterval = 500 * time.Millisecond
	}
	if policy.ElectionWait <= 0 {
		policy.ElectionWait = leaderElectionWaitDuration
	}
	return policy
}

// DemoteAllBluePods demotes all Blue peers to non-voters after leadership moves
// to Green.
func DemoteAllBluePods(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, client RaftPeerDemoter) error {
	if client == nil {
		return fmt.Errorf("client is required to demote Blue pods")
	}

	for _, i := range ReplicaOrdinals(cfg.ClusterReplicas) {
		bluePodName := RevisionPodName(cfg.ClusterName, cfg.BlueRevision, i)
		logger.V(1).Info("Demoting Blue pod to non-voter", "pod_name", bluePodName)
		if err := client.DemoteRaftPeer(ctx, bluePodName); err != nil {
			if IsBenignDemoteError(err) {
				logger.V(1).Info("Blue pod already non-voter after leader transfer", "pod_name", bluePodName)
				continue
			}
			return fmt.Errorf("failed to demote Blue pod %q to non-voter: %w", bluePodName, err)
		}
	}
	return nil
}
