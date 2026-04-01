package raftops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ReasonPrimaryLeaderFound             = "reason_primary_leader_found"
	ReasonPrimaryLeaderNotFound          = "reason_primary_leader_not_found"
	ReasonFallbackLeaderFound            = "reason_fallback_leader_found"
	ReasonFallbackLeaderNotFound         = "reason_fallback_leader_not_found"
	ReasonElectionNewLeaderFound         = "reason_election_new_leader_found"
	ReasonElectionSameLeaderSeen         = "reason_election_same_leader_seen"
	ReasonDemoteFatal                    = "reason_demote_fatal"
	ReasonStepDownFatal                  = "reason_stepdown_fatal"
	ReasonLeaderTransferStateFailed      = "reason_leader_transfer_state_failed"
	ReasonLeaderTransferRetriesExhausted = "reason_leader_transfer_retries_exhausted"
	ReasonContextCanceled                = "reason_context_canceled"
	ReasonDeadlineExceeded               = "reason_deadline_exceeded"
	ReasonElectionTimeout                = "reason_election_timeout"
)

const (
	DecisionPathPrimarySuccess               = "primary_success"
	DecisionPathPrimaryFailedFallbackSuccess = "primary_failed_fallback_success"
	DecisionPathPrimaryFailedNoFallback      = "primary_failed_no_fallback"
	DecisionPathPrimaryFailedFallbackFailed  = "primary_failed_fallback_failed"
	DecisionPathContextCanceled              = "context_canceled"
	DecisionPathDeadlineExceeded             = "deadline_exceeded"
	DecisionPathElectionTimeout              = "election_timeout"
	DecisionPathElectionObservedNewLeader    = "election_observed_new_leader"
	DecisionPathElectionObservedSameLeader   = "election_observed_same_leader"
)

// LeaderSearchPolicy configures how a leader search falls back.
type LeaderSearchPolicy struct {
	PrimaryRevision  string
	FallbackRevision string
	PrimaryLabel     string
	FallbackLabel    string
	AllowFallback    bool
}

// RetryPolicy configures retries for executor-side operations.
type RetryPolicy struct {
	MaxAttempts     int
	AttemptInterval time.Duration
	ElectionWait    time.Duration
}

// BenignErrorClassification determines whether an error should be treated as
// benign, retryable, or fatal.
type BenignErrorClassification string

const (
	BenignErrorClassificationBenign    BenignErrorClassification = "benign"
	BenignErrorClassificationRetryable BenignErrorClassification = "retryable"
	BenignErrorClassificationFatal     BenignErrorClassification = "fatal"
)

// LeaderSearchOutcome captures the result of a policy-driven leader search.
type LeaderSearchOutcome struct {
	Value         string
	DecisionPath  string
	ReasonCode    string
	AttemptsUsed  int
	PrimaryError  error
	FallbackError error
}

// LeaderElectionOutcome captures the outcome of waiting for leader election.
type LeaderElectionOutcome struct {
	Value        string
	DecisionPath string
	ReasonCode   string
	WaitError    error
}

type executorReasonedError struct {
	ReasonCode string
	Message    string
	Err        error
}

func (e *executorReasonedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		if e.Err == nil {
			return ""
		}
		return e.Err.Error()
	}
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *executorReasonedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewExecutorReasonedError annotates an executor-side error with a reason code.
func NewExecutorReasonedError(reasonCode string, message string, err error) error {
	return &executorReasonedError{
		ReasonCode: reasonCode,
		Message:    message,
		Err:        err,
	}
}

// ReasonCodeFromError extracts the executor reason code when present.
func ReasonCodeFromError(err error) string {
	if err == nil {
		return ""
	}

	var reasoned *executorReasonedError
	if errors.As(err, &reasoned) {
		return reasoned.ReasonCode
	}
	return ""
}

// ReasonCodeFromContextError maps context cancellation errors to reason codes.
func ReasonCodeFromContextError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return ReasonContextCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonDeadlineExceeded
	default:
		return ""
	}
}

// DecisionPathFromReasonCode maps reason codes back to leader search decision
// paths for logging and tests.
func DecisionPathFromReasonCode(reasonCode string) string {
	switch reasonCode {
	case ReasonContextCanceled:
		return DecisionPathContextCanceled
	case ReasonDeadlineExceeded:
		return DecisionPathDeadlineExceeded
	case ReasonElectionTimeout:
		return DecisionPathElectionTimeout
	default:
		return DecisionPathPrimaryFailedFallbackFailed
	}
}

// NewLeaderSearchPolicy constructs a leader search policy from primary and
// fallback revisions.
func NewLeaderSearchPolicy(
	primaryRevision string,
	fallbackRevision string,
	primaryLabel string,
	fallbackLabel string,
) LeaderSearchPolicy {
	allowFallback := strings.TrimSpace(fallbackRevision) != "" && fallbackRevision != primaryRevision
	return LeaderSearchPolicy{
		PrimaryRevision:  primaryRevision,
		FallbackRevision: fallbackRevision,
		PrimaryLabel:     primaryLabel,
		FallbackLabel:    fallbackLabel,
		AllowFallback:    allowFallback,
	}
}

// MaxLeaderSearchAttempts returns the number of revision scans a policy allows.
func MaxLeaderSearchAttempts(policy LeaderSearchPolicy) int {
	if policy.AllowFallback {
		return 2
	}
	return 1
}

// LeaderFinder finds a leader URL for the given revision.
type LeaderFinder func(context.Context, *ExecutorConfig, string) (string, error)

// ResolveLeaderWithPolicyUsing resolves a leader using the provided search
// policy and finder implementation.
func ResolveLeaderWithPolicyUsing(
	ctx context.Context,
	cfg *ExecutorConfig,
	policy LeaderSearchPolicy,
	finder LeaderFinder,
) LeaderSearchOutcome {
	outcome := LeaderSearchOutcome{
		DecisionPath: DecisionPathPrimaryFailedNoFallback,
		ReasonCode:   ReasonPrimaryLeaderNotFound,
	}

	primaryURL, primaryErr := finder(ctx, cfg, policy.PrimaryRevision)
	outcome.AttemptsUsed = 1
	if primaryErr == nil {
		outcome.Value = primaryURL
		outcome.DecisionPath = DecisionPathPrimarySuccess
		outcome.ReasonCode = ReasonPrimaryLeaderFound
		return outcome
	}
	outcome.PrimaryError = primaryErr
	if reasonCode := ReasonCodeFromContextError(primaryErr); reasonCode != "" {
		outcome.DecisionPath = DecisionPathFromReasonCode(reasonCode)
		outcome.ReasonCode = reasonCode
	}

	if !policy.AllowFallback {
		return outcome
	}

	fallbackURL, fallbackErr := finder(ctx, cfg, policy.FallbackRevision)
	outcome.AttemptsUsed = 2
	if fallbackErr == nil {
		outcome.Value = fallbackURL
		outcome.DecisionPath = DecisionPathPrimaryFailedFallbackSuccess
		outcome.ReasonCode = ReasonFallbackLeaderFound
		return outcome
	}

	outcome.FallbackError = fallbackErr
	outcome.DecisionPath = DecisionPathPrimaryFailedFallbackFailed
	outcome.ReasonCode = ReasonFallbackLeaderNotFound
	if reasonCode := ReasonCodeFromContextError(fallbackErr); reasonCode != "" {
		outcome.DecisionPath = DecisionPathFromReasonCode(reasonCode)
		outcome.ReasonCode = reasonCode
	}
	return outcome
}
