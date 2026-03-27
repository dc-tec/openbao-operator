package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func findInitialLeader(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig) (string, error) {
	leaderURL, err := findPreferredLeaderWithFallback(
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

type leaderTransferClient interface {
	ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error)
	DemoteRaftPeer(context.Context, string) error
	StepDown(context.Context) error
}

type leaderTransferClientResolver func(context.Context, string) (leaderTransferClient, error)

type leaderTransferWaitFunc func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error)

const (
	leaderTransferStateResolveCurrentLeader = "ResolveCurrentLeader"
	leaderTransferStateInspectRaftConfig    = "InspectRaftConfig"
	leaderTransferStateBiasElection         = "BiasElection"
	leaderTransferStateStepDown             = "StepDown"
	leaderTransferStateAwaitNewLeader       = "AwaitNewLeader"
	leaderTransferStateValidateGreenLeader  = "ValidateGreenLeader"
)

func ensureGreenLeaderBySteppingDownBlue(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	factory *openbao.ClientFactory,
	leaderURL string,
) (leaderTransferClient, error) {
	resolver := func(ctx context.Context, leaderURL string) (leaderTransferClient, error) {
		return clientForLeaderURL(ctx, cfg, factory, leaderURL)
	}
	return ensureGreenLeaderBySteppingDownBlueWithFuncs(
		ctx,
		logger,
		cfg,
		leaderURL,
		retryPolicy{MaxAttempts: defaultLeaderTransferMaxRetries},
		resolver,
		waitForNewLeaderURL,
	)
}

func ensureGreenLeaderBySteppingDownBlueWithFuncs(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	leaderURL string,
	policy retryPolicy,
	resolveClient leaderTransferClientResolver,
	waitForLeader leaderTransferWaitFunc,
) (leaderTransferClient, error) {
	policy = normalizeLeaderTransferRetryPolicy(policy)
	bluePrefix := fmt.Sprintf("%s-%s-", cfg.ClusterName, cfg.BlueRevision)

	for _, attempt := range attemptOrdinals(policy.MaxAttempts) {
		attemptNumber := attempt + 1
		state := leaderTransferStateResolveCurrentLeader
		var client leaderTransferClient
		var config *portopenbao.RaftConfigurationResponse
		var leaderID string
		var leaderIsBlue bool

		for {
			switch state {
			case leaderTransferStateResolveCurrentLeader:
				resolvedClient, err := resolveClient(ctx, leaderURL)
				if err != nil {
					return nil, newExecutorReasonedError(reasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				client = resolvedClient
				state = leaderTransferStateInspectRaftConfig

			case leaderTransferStateInspectRaftConfig:
				currentConfig, err := client.ReadRaftConfiguration(ctx)
				if err != nil {
					return nil, newExecutorReasonedError(reasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				config = currentConfig
				leaderID, leaderIsBlue = raftLeaderInfo(config, bluePrefix)
				if leaderID == "" {
					return nil, newExecutorReasonedError(reasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), errors.New("raft leader not found in configuration"))
				}
				if !leaderIsBlue {
					logger.Info("Leader is not Blue (assumed Green), proceeding to demotion", "state", leaderTransferStateValidateGreenLeader, "attempt", attemptNumber, "max_retries", policy.MaxAttempts)
					return client, nil
				}

				logger.Info("Current leader is Blue", "leader_id", leaderID, "state", state, "attempt", attemptNumber, "max_retries", policy.MaxAttempts)
				state = leaderTransferStateBiasElection

			case leaderTransferStateBiasElection:
				if err := demoteBlueVotersExceptLeader(ctx, logger, cfg, client, config, leaderID, bluePrefix); err != nil {
					return nil, newExecutorReasonedError(reasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				state = leaderTransferStateStepDown

			case leaderTransferStateStepDown:
				classification, err := stepDownLeader(ctx, logger, client)
				if err != nil && classification == benignErrorClassificationFatal {
					return nil, newExecutorReasonedError(reasonStepDownFatal, fmt.Sprintf("leader transfer state %s failed", state), err)
				}
				state = leaderTransferStateAwaitNewLeader

			case leaderTransferStateAwaitNewLeader:
				newLeaderURL, err := waitForLeader(ctx, logger, cfg, leaderURL)
				if err != nil {
					return nil, newExecutorReasonedError(reasonLeaderTransferStateFailed, fmt.Sprintf("leader transfer state %s failed", state), err)
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

	return nil, newExecutorReasonedError(
		reasonLeaderTransferRetriesExhausted,
		fmt.Sprintf("failed to transfer leadership to Green node after %d attempts", policy.MaxAttempts),
		nil,
	)
}

func normalizeLeaderTransferRetryPolicy(policy retryPolicy) retryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaultLeaderTransferMaxRetries
	}
	return policy
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

func raftLeaderInfo(config *portopenbao.RaftConfigurationResponse, bluePrefix string) (string, bool) {
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
		if !isBlueRaftServer(server.NodeID, server.Address, bluePrefix) {
			continue
		}

		logger.Info("Demoting Blue peer before step-down to bias election", "node_id", server.NodeID)
		if err := client.DemoteRaftPeer(ctx, server.NodeID); err != nil {
			classification := classifyDemoteError(err)
			if classification == benignErrorClassificationBenign {
				logger.V(1).Info("Blue peer already non-voter before step-down", "node_id", server.NodeID)
				continue
			}
			if classification == benignErrorClassificationRetryable {
				logger.Error(err, "Failed to demote Blue peer before step-down; continuing", "node_id", server.NodeID, "cluster_replicas", cfg.ClusterReplicas, "error_classification", classification)
				continue
			}

			return newExecutorReasonedError(
				reasonDemoteFatal,
				fmt.Sprintf("failed to demote Blue peer %q before step-down", server.NodeID),
				err,
			)
		}
	}

	return nil
}

func stepDownLeader(ctx context.Context, logger logr.Logger, client leaderTransferClient) (benignErrorClassification, error) {
	logger.Info("Stepping down Blue leader to transfer leadership to Green")
	if err := client.StepDown(ctx); err != nil {
		classification := classifyStepDownError(err)
		logger.Error(err, "Failed to step down leader", "error_classification", classification)
		return classification, err
	}
	return benignErrorClassificationBenign, nil
}

func waitForNewLeaderURL(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, previousLeaderURL string) (string, error) {
	return waitForNewLeaderURLWithFuncs(ctx, logger, cfg, previousLeaderURL, waitForLeaderElectionOutcome, findAnyLeader)
}

type leaderElectionWaitFunc func(context.Context, *ExecutorConfig, string) leaderElectionOutcome
type leaderFallbackResolver func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error)

func waitForNewLeaderURLWithFuncs(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	previousLeaderURL string,
	waitFn leaderElectionWaitFunc,
	fallbackFn leaderFallbackResolver,
) (string, error) {
	logger.Info("Waiting for new leader election...")

	waitOutcome := waitFn(ctx, cfg, previousLeaderURL)
	if ctxErr := ctx.Err(); ctxErr != nil {
		reasonCode := reasonCodeFromContextError(ctxErr)
		if reasonCode == "" {
			reasonCode = reasonContextCanceled
		}
		return "", newExecutorReasonedError(reasonCode, "failed while waiting for new leader election", ctxErr)
	}
	if waitOutcome.WaitError != nil && !errors.Is(waitOutcome.WaitError, context.DeadlineExceeded) && !errors.Is(waitOutcome.WaitError, context.Canceled) {
		reasonCode := reasonCodeFromError(waitOutcome.WaitError)
		if reasonCode == "" {
			reasonCode = reasonElectionTimeout
		}
		return "", newExecutorReasonedError(reasonCode, "failed while waiting for new leader election", waitOutcome.WaitError)
	}

	logger.Info(
		"Leader election wait completed",
		"decision_path", waitOutcome.DecisionPath,
		"reason_code", waitOutcome.ReasonCode,
		"leader_url", waitOutcome.Value,
	)
	if waitOutcome.DecisionPath == decisionPathElectionObservedNewLeader && strings.TrimSpace(waitOutcome.Value) != "" {
		logger.Info("New leader found", "leader_url", waitOutcome.Value)
		return waitOutcome.Value, nil
	}
	if waitOutcome.DecisionPath == decisionPathElectionObservedSameLeader && strings.TrimSpace(waitOutcome.Value) != "" {
		logger.Info("Leader election retained previous leader; proceeding with observed leader", "leader_url", waitOutcome.Value)
		return waitOutcome.Value, nil
	}

	logger.Info("Finding new leader via fallback search...")
	leaderURL, findErr := fallbackFn(ctx, logger, cfg, cfg.GreenRevision, cfg.BlueRevision)
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

type leaderOnceFinder func(context.Context, *ExecutorConfig, string) (string, bool)

func waitForLeaderElectionOutcome(ctx context.Context, cfg *ExecutorConfig, previousLeaderURL string) leaderElectionOutcome {
	return waitForLeaderElectionWithFinderAndPolicy(
		ctx,
		cfg,
		previousLeaderURL,
		retryPolicy{
			AttemptInterval: 500 * time.Millisecond,
			ElectionWait:    leaderElectionWaitDuration,
		},
		findLeaderOnce,
	)
}

func waitForLeaderElectionWithFinderAndPolicy(
	ctx context.Context,
	cfg *ExecutorConfig,
	previousLeaderURL string,
	policy retryPolicy,
	finder leaderOnceFinder,
) leaderElectionOutcome {
	policy = normalizeElectionRetryPolicy(policy)

	outcome := leaderElectionOutcome{
		DecisionPath: decisionPathElectionTimeout,
		ReasonCode:   reasonElectionTimeout,
	}
	lastObservedLeaderURL := ""

	err := wait.PollUntilContextTimeout(ctx, policy.AttemptInterval, policy.ElectionWait, true, func(ctx context.Context) (bool, error) {
		if url, ok := finder(ctx, cfg, cfg.GreenRevision); ok {
			outcome.Value = url
			outcome.DecisionPath = decisionPathElectionObservedNewLeader
			outcome.ReasonCode = reasonElectionNewLeaderFound
			return true, nil
		}

		if url, ok := finder(ctx, cfg, cfg.BlueRevision); ok {
			if url != previousLeaderURL {
				outcome.Value = url
				outcome.DecisionPath = decisionPathElectionObservedNewLeader
				outcome.ReasonCode = reasonElectionNewLeaderFound
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
			outcome.DecisionPath = decisionPathElectionObservedSameLeader
			outcome.ReasonCode = reasonElectionSameLeaderSeen
		}
		outcome.WaitError = newExecutorReasonedError(reasonElectionTimeout, "leader election did not converge within wait duration", err)
		return outcome
	}

	if reasonCode := reasonCodeFromContextError(err); reasonCode != "" {
		outcome.DecisionPath = decisionPathFromReasonCode(reasonCode)
		outcome.ReasonCode = reasonCode
		outcome.WaitError = newExecutorReasonedError(reasonCode, "leader election was interrupted", err)
		return outcome
	}

	outcome.WaitError = newExecutorReasonedError(reasonElectionTimeout, "leader election did not converge within wait duration", err)
	return outcome
}

func normalizeElectionRetryPolicy(policy retryPolicy) retryPolicy {
	if policy.AttemptInterval <= 0 {
		policy.AttemptInterval = 500 * time.Millisecond
	}
	if policy.ElectionWait <= 0 {
		policy.ElectionWait = leaderElectionWaitDuration
	}
	return policy
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
