package upgrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func loginJWT(ctx context.Context, cfg *ExecutorConfig, baseURL string) (string, error) {
	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()

	return factory.LoginJWT(ctx, baseURL, cfg.JWTAuthRole, cfg.JWTToken)
}

func findLeader(ctx context.Context, cfg *ExecutorConfig, revision string) (string, error) {
	return resolveLeaderWithRetry(
		ctx,
		cfg,
		revision,
		retryPolicy{
			MaxAttempts:     defaultLeaderSearchMaxAttempts,
			AttemptInterval: defaultLeaderSearchWaitInterval,
		},
	)
}

func resolveLeaderWithRetry(
	ctx context.Context,
	cfg *ExecutorConfig,
	revision string,
	policy retryPolicy,
) (string, error) {
	policy = normalizeRetryPolicy(policy)

	factory, cleanup, err := newOpenBaoClientFactory(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()

	for range attemptOrdinals(policy.MaxAttempts) {
		if url, found := findLeaderInSingleScan(ctx, cfg, revision, factory); found {
			return url, nil
		}

		if policy.AttemptInterval <= 0 {
			continue
		}

		timer := time.NewTimer(policy.AttemptInterval)
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

func findLeaderInSingleScan(
	ctx context.Context,
	cfg *ExecutorConfig,
	revision string,
	factory *openbao.ClientFactory,
) (string, bool) {
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

func normalizeRetryPolicy(policy retryPolicy) retryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = singleLeaderSearchAttempt
	}
	if policy.AttemptInterval < 0 {
		policy.AttemptInterval = 0
	}
	return policy
}

type leaderFinder func(context.Context, *ExecutorConfig, string) (string, error)

func findPreferredLeaderWithFallback(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	preferredRevision string,
	fallbackRevision string,
	preferredLabel string,
	fallbackLabel string,
) (string, error) {
	policy := newLeaderSearchPolicy(preferredRevision, fallbackRevision, preferredLabel, fallbackLabel)
	return findLeaderWithPolicy(ctx, logger, cfg, policy)
}

func findAnyLeader(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	firstRevision string,
	secondRevision string,
) (string, error) {
	return findPreferredLeaderWithFallback(ctx, logger, cfg, firstRevision, secondRevision, "first", "second")
}

func findLeaderWithPolicy(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	policy leaderSearchPolicy,
) (string, error) {
	return findLeaderWithPolicyUsing(ctx, logger, cfg, policy, findLeader)
}

func findLeaderWithPolicyUsing(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	policy leaderSearchPolicy,
	finder leaderFinder,
) (string, error) {
	outcome := resolveLeaderWithPolicyUsing(ctx, cfg, policy, finder)
	logger.Info(
		"Leader search completed",
		"decision_path", outcome.DecisionPath,
		"reason_code", outcome.ReasonCode,
		"attempt", outcome.AttemptsUsed,
		"max_attempts", maxLeaderSearchAttempts(policy),
		"primary_revision", policy.PrimaryRevision,
		"fallback_revision", policy.FallbackRevision,
	)
	if outcome.Value != "" {
		return outcome.Value, nil
	}

	if policy.AllowFallback {
		logger.Info(
			fmt.Sprintf("Failed to find leader among %s pods, checking %s pods", policy.PrimaryLabel, policy.FallbackLabel),
			"error", outcome.PrimaryError,
			"decision_path", outcome.DecisionPath,
			"reason_code", outcome.ReasonCode,
		)
		return "", newExecutorReasonedError(
			outcome.ReasonCode,
			fmt.Sprintf("failed to find leader (checked %s and %s)", policy.PrimaryLabel, policy.FallbackLabel),
			outcome.FallbackError,
		)
	}

	return "", newExecutorReasonedError(
		outcome.ReasonCode,
		fmt.Sprintf("failed to find leader among %s pods", policy.PrimaryLabel),
		outcome.PrimaryError,
	)
}

func findLeaderOnce(ctx context.Context, cfg *ExecutorConfig, revision string) (string, bool) {
	leaderURL, err := resolveLeaderWithRetry(
		ctx,
		cfg,
		revision,
		retryPolicy{
			MaxAttempts: singleLeaderSearchAttempt,
		},
	)
	if err != nil {
		return "", false
	}
	return leaderURL, true
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

func classifyJoinError(err error) benignErrorClassification {
	if err == nil {
		return benignErrorClassificationBenign
	}
	if isBenignJoinError(err) {
		return benignErrorClassificationBenign
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "permission denied") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "unauthorized") {
		return benignErrorClassificationFatal
	}

	return benignErrorClassificationFatal
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

func classifyDemoteError(err error) benignErrorClassification {
	if err == nil {
		return benignErrorClassificationBenign
	}
	if isBenignDemoteError(err) {
		return benignErrorClassificationBenign
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "permission denied") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "unauthorized") {
		return benignErrorClassificationFatal
	}

	return benignErrorClassificationRetryable
}

func classifyStepDownError(err error) benignErrorClassification {
	if err == nil {
		return benignErrorClassificationBenign
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "permission denied") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "unauthorized") {
		return benignErrorClassificationFatal
	}

	return benignErrorClassificationRetryable
}

func newOpenBaoClientFactory(cfg *ExecutorConfig) (*openbao.ClientFactory, func(), error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("config is required")
	}

	mgr := openbao.NewClientManager(portopenbao.ClientConfig{
		ClusterKey:                     fmt.Sprintf("%s/%s", cfg.ClusterNamespace, cfg.ClusterName),
		CACert:                         cfg.TLSCACert,
		TLSServerName:                  cfg.TLSServerName,
		RateLimitQPS:                   cfg.ClientQPS,
		RateLimitBurst:                 cfg.ClientBurst,
		CircuitBreakerFailureThreshold: cfg.ClientCircuitBreakerFailureThreshold,
		CircuitBreakerOpenDuration:     cfg.ClientCircuitBreakerOpenDuration,
	})

	factory := mgr.FactoryFor(fmt.Sprintf("%s/%s", cfg.ClusterNamespace, cfg.ClusterName), cfg.TLSCACert)
	return factory, mgr.Close, nil
}
