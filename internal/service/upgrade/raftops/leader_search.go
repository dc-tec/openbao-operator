package raftops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/go-logr/logr"
)

const (
	defaultLeaderSearchMaxAttempts  = 10
	defaultLeaderSearchWaitInterval = 2 * time.Second
	singleLeaderSearchAttempt       = 1
)

// LoginJWT authenticates the executor against the target OpenBao endpoint.
func LoginJWT(ctx context.Context, cfg *ExecutorConfig, baseURL string) (string, error) {
	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()

	return factory.LoginJWT(ctx, baseURL, cfg.JWTAuthRole, cfg.JWTToken)
}

// FindLeader finds a leader for the given revision with the default retry
// policy.
func FindLeader(ctx context.Context, cfg *ExecutorConfig, revision string) (string, error) {
	return ResolveLeaderWithRetry(
		ctx,
		cfg,
		revision,
		RetryPolicy{
			MaxAttempts:     defaultLeaderSearchMaxAttempts,
			AttemptInterval: defaultLeaderSearchWaitInterval,
		},
	)
}

// ResolveLeaderWithRetry resolves a leader using the supplied retry policy.
func ResolveLeaderWithRetry(
	ctx context.Context,
	cfg *ExecutorConfig,
	revision string,
	policy RetryPolicy,
) (string, error) {
	policy = NormalizeRetryPolicy(policy)

	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		return "", err
	}
	defer cleanup()

	for range AttemptOrdinals(policy.MaxAttempts) {
		if url, found := FindLeaderInSingleScan(ctx, cfg, revision, factory); found {
			return url, nil
		}

		if policy.AttemptInterval <= 0 {
			continue
		}

		timer := time.NewTimer(policy.AttemptInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			reasonCode := ReasonCodeFromContextError(ctx.Err())
			if reasonCode == "" {
				reasonCode = ReasonContextCanceled
			}
			return "", NewExecutorReasonedError(reasonCode, "context cancelled while finding leader", ctx.Err())
		case <-timer.C:
		}
	}

	return "", NewExecutorReasonedError(ReasonPrimaryLeaderNotFound, fmt.Sprintf("no leader found among %d pods", cfg.ClusterReplicas), nil)
}

// FindLeaderInSingleScan scans each pod once and returns the first leader.
func FindLeaderInSingleScan(
	ctx context.Context,
	cfg *ExecutorConfig,
	revision string,
	factory *openbao.ClientFactory,
) (string, bool) {
	for _, i := range ReplicaOrdinals(cfg.ClusterReplicas) {
		url := PodURL(cfg, revision, i)
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

// NormalizeRetryPolicy normalizes leader search retry defaults.
func NormalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = singleLeaderSearchAttempt
	}
	if policy.AttemptInterval < 0 {
		policy.AttemptInterval = 0
	}
	return policy
}

// FindPreferredLeaderWithFallback finds a leader on the preferred revision and
// optionally falls back to another revision.
func FindPreferredLeaderWithFallback(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	preferredRevision string,
	fallbackRevision string,
	preferredLabel string,
	fallbackLabel string,
) (string, error) {
	policy := NewLeaderSearchPolicy(preferredRevision, fallbackRevision, preferredLabel, fallbackLabel)
	return FindLeaderWithPolicy(ctx, logger, cfg, policy)
}

// FindAnyLeader tries the first revision and then the second revision.
func FindAnyLeader(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	firstRevision string,
	secondRevision string,
) (string, error) {
	return FindPreferredLeaderWithFallback(ctx, logger, cfg, firstRevision, secondRevision, "first", "second")
}

// FindLeaderWithPolicy resolves a leader using the provided search policy.
func FindLeaderWithPolicy(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	policy LeaderSearchPolicy,
) (string, error) {
	return FindLeaderWithPolicyUsing(ctx, logger, cfg, policy, FindLeader)
}

// FindLeaderWithPolicyUsing resolves a leader using a custom finder.
func FindLeaderWithPolicyUsing(
	ctx context.Context,
	logger logr.Logger,
	cfg *ExecutorConfig,
	policy LeaderSearchPolicy,
	finder LeaderFinder,
) (string, error) {
	outcome := ResolveLeaderWithPolicyUsing(ctx, cfg, policy, finder)
	logger.Info(
		"Leader search completed",
		"decision_path", outcome.DecisionPath,
		"reason_code", outcome.ReasonCode,
		"attempt", outcome.AttemptsUsed,
		"max_attempts", MaxLeaderSearchAttempts(policy),
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
		return "", NewExecutorReasonedError(
			outcome.ReasonCode,
			fmt.Sprintf("failed to find leader (checked %s and %s)", policy.PrimaryLabel, policy.FallbackLabel),
			outcome.FallbackError,
		)
	}

	return "", NewExecutorReasonedError(
		outcome.ReasonCode,
		fmt.Sprintf("failed to find leader among %s pods", policy.PrimaryLabel),
		outcome.PrimaryError,
	)
}

// FindLeaderOnce performs a single leader scan.
func FindLeaderOnce(ctx context.Context, cfg *ExecutorConfig, revision string) (string, bool) {
	leaderURL, err := ResolveLeaderWithRetry(
		ctx,
		cfg,
		revision,
		RetryPolicy{MaxAttempts: singleLeaderSearchAttempt},
	)
	if err != nil {
		return "", false
	}
	return leaderURL, true
}

// PodURL returns the per-pod OpenBao API URL for the given revision and ordinal.
func PodURL(cfg *ExecutorConfig, revision string, ordinal int32) string {
	podName := RevisionPodName(cfg.ClusterName, revision, ordinal)
	host := fmt.Sprintf("%s.%s.%s.svc", podName, cfg.ClusterName, cfg.ClusterNamespace)
	return fmt.Sprintf("https://%s:%d", host, constants.PortAPI)
}

// RevisionPodName returns the expected pod name for the revision and ordinal.
func RevisionPodName(clusterName string, revision string, ordinal int32) string {
	if revision == "" {
		return fmt.Sprintf("%s-%d", clusterName, ordinal)
	}
	return fmt.Sprintf("%s-%s-%d", clusterName, revision, ordinal)
}

// RaftServerMatchesRevision reports whether a server belongs to the given revision.
func RaftServerMatchesRevision(nodeID string, address string, clusterName string, revision string, replicas int32) bool {
	for _, i := range ReplicaOrdinals(replicas) {
		podName := RevisionPodName(clusterName, revision, i)
		if nodeID == podName || strings.Contains(address, podName) {
			return true
		}
	}
	return false
}

// ReplicaOrdinals expands a replica count into ordinal indexes.
func ReplicaOrdinals(replicas int32) []int32 {
	if replicas <= 0 {
		return nil
	}
	ordinals := make([]int32, 0, replicas)
	for i := int32(0); i < replicas; i++ {
		ordinals = append(ordinals, i)
	}
	return ordinals
}

// AttemptOrdinals expands an attempt count into indexes.
func AttemptOrdinals(maxAttempts int) []int {
	if maxAttempts <= 0 {
		return nil
	}
	ordinals := make([]int, 0, maxAttempts)
	for i := 0; i < maxAttempts; i++ {
		ordinals = append(ordinals, i)
	}
	return ordinals
}

// IsBenignJoinError reports whether a join failure is a harmless already-joined
// condition.
func IsBenignJoinError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, portopenbao.ErrAlreadyJoined)
}

// ClassifyJoinError classifies a join failure.
func ClassifyJoinError(err error) BenignErrorClassification {
	if err == nil {
		return BenignErrorClassificationBenign
	}
	if IsBenignJoinError(err) {
		return BenignErrorClassificationBenign
	}

	if isOpenBaoAuthzError(err) {
		return BenignErrorClassificationFatal
	}

	return BenignErrorClassificationFatal
}

// IsBenignDemoteError reports whether a demote failure is a harmless
// already-non-voter condition.
func IsBenignDemoteError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, portopenbao.ErrAlreadyNonVoter)
}

// ClassifyDemoteError classifies a demote failure.
func ClassifyDemoteError(err error) BenignErrorClassification {
	if err == nil {
		return BenignErrorClassificationBenign
	}
	if IsBenignDemoteError(err) {
		return BenignErrorClassificationBenign
	}

	if isOpenBaoAuthzError(err) {
		return BenignErrorClassificationFatal
	}

	return BenignErrorClassificationRetryable
}

// ClassifyStepDownError classifies a step-down failure.
func ClassifyStepDownError(err error) BenignErrorClassification {
	if err == nil {
		return BenignErrorClassificationBenign
	}

	if isOpenBaoAuthzError(err) {
		return BenignErrorClassificationFatal
	}

	return BenignErrorClassificationRetryable
}

func isOpenBaoAuthzError(err error) bool {
	return portopenbao.IsStatus(err, http.StatusUnauthorized) || portopenbao.IsStatus(err, http.StatusForbidden)
}

// NewOpenBaoClientFactory constructs a client factory for executor-side pod
// coordination.
func NewOpenBaoClientFactory(cfg *ExecutorConfig) (*openbao.ClientFactory, func(), error) {
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
