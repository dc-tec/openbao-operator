package upgrade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	reasonPrimaryLeaderFound     = "reason_primary_leader_found"
	reasonPrimaryLeaderNotFound  = "reason_primary_leader_not_found"
	reasonFallbackLeaderFound    = "reason_fallback_leader_found"
	reasonFallbackLeaderNotFound = "reason_fallback_leader_not_found"
	reasonContextCanceled        = "reason_context_canceled"
	reasonDeadlineExceeded       = "reason_deadline_exceeded"
	reasonElectionTimeout        = "reason_election_timeout"
)

const (
	decisionPathPrimarySuccess               = "primary_success"
	decisionPathPrimaryFailedFallbackSuccess = "primary_failed_fallback_success"
	decisionPathPrimaryFailedNoFallback      = "primary_failed_no_fallback"
	decisionPathPrimaryFailedFallbackFailed  = "primary_failed_fallback_failed"
	decisionPathContextCanceled              = "context_canceled"
	decisionPathDeadlineExceeded             = "deadline_exceeded"
	decisionPathElectionTimeout              = "election_timeout"
)

type leaderSearchPolicy struct {
	PrimaryRevision  string
	FallbackRevision string
	PrimaryLabel     string
	FallbackLabel    string
	AllowFallback    bool
}

type retryPolicy struct {
	MaxAttempts     int
	AttemptInterval time.Duration
	ElectionWait    time.Duration
}

type benignErrorClassification string

const (
	benignErrorClassificationBenign    benignErrorClassification = "benign"
	benignErrorClassificationRetryable benignErrorClassification = "retryable"
	benignErrorClassificationFatal     benignErrorClassification = "fatal"
)

type leaderSearchOutcome struct {
	Value         string
	DecisionPath  string
	ReasonCode    string
	AttemptsUsed  int
	PrimaryError  error
	FallbackError error
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

func newExecutorReasonedError(reasonCode string, message string, err error) error {
	return &executorReasonedError{
		ReasonCode: reasonCode,
		Message:    message,
		Err:        err,
	}
}

func reasonCodeFromError(err error) string {
	if err == nil {
		return ""
	}

	var reasoned *executorReasonedError
	if errors.As(err, &reasoned) {
		return reasoned.ReasonCode
	}
	return ""
}

func reasonCodeFromContextError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return reasonContextCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return reasonDeadlineExceeded
	default:
		return ""
	}
}

func decisionPathFromReasonCode(reasonCode string) string {
	switch reasonCode {
	case reasonContextCanceled:
		return decisionPathContextCanceled
	case reasonDeadlineExceeded:
		return decisionPathDeadlineExceeded
	case reasonElectionTimeout:
		return decisionPathElectionTimeout
	default:
		return decisionPathPrimaryFailedFallbackFailed
	}
}

func newLeaderSearchPolicy(
	primaryRevision string,
	fallbackRevision string,
	primaryLabel string,
	fallbackLabel string,
) leaderSearchPolicy {
	allowFallback := strings.TrimSpace(fallbackRevision) != "" && fallbackRevision != primaryRevision
	return leaderSearchPolicy{
		PrimaryRevision:  primaryRevision,
		FallbackRevision: fallbackRevision,
		PrimaryLabel:     primaryLabel,
		FallbackLabel:    fallbackLabel,
		AllowFallback:    allowFallback,
	}
}

func resolveLeaderWithPolicyUsing(
	ctx context.Context,
	cfg *ExecutorConfig,
	policy leaderSearchPolicy,
	finder leaderFinder,
) leaderSearchOutcome {
	outcome := leaderSearchOutcome{
		DecisionPath: decisionPathPrimaryFailedNoFallback,
		ReasonCode:   reasonPrimaryLeaderNotFound,
	}

	primaryURL, primaryErr := finder(ctx, cfg, policy.PrimaryRevision)
	outcome.AttemptsUsed = 1
	if primaryErr == nil {
		outcome.Value = primaryURL
		outcome.DecisionPath = decisionPathPrimarySuccess
		outcome.ReasonCode = reasonPrimaryLeaderFound
		return outcome
	}
	outcome.PrimaryError = primaryErr
	if reasonCode := reasonCodeFromContextError(primaryErr); reasonCode != "" {
		outcome.DecisionPath = decisionPathFromReasonCode(reasonCode)
		outcome.ReasonCode = reasonCode
	}

	if !policy.AllowFallback {
		return outcome
	}

	fallbackURL, fallbackErr := finder(ctx, cfg, policy.FallbackRevision)
	outcome.AttemptsUsed = 2
	if fallbackErr == nil {
		outcome.Value = fallbackURL
		outcome.DecisionPath = decisionPathPrimaryFailedFallbackSuccess
		outcome.ReasonCode = reasonFallbackLeaderFound
		return outcome
	}

	outcome.FallbackError = fallbackErr
	outcome.DecisionPath = decisionPathPrimaryFailedFallbackFailed
	outcome.ReasonCode = reasonFallbackLeaderNotFound
	if reasonCode := reasonCodeFromContextError(fallbackErr); reasonCode != "" {
		outcome.DecisionPath = decisionPathFromReasonCode(reasonCode)
		outcome.ReasonCode = reasonCode
	}
	return outcome
}
