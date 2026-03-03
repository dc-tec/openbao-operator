package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
)

func TestRunExecutor(t *testing.T) {
	t.Parallel()

	canceledCtx := canceledContext()

	tests := []struct {
		name     string
		cfg      *ExecutorConfig
		ctx      context.Context
		wantErr  string
		wantOkay bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			ctx:     context.Background(),
			wantErr: "config is required",
		},
		{
			name: "unsupported action",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorAction("unsupported-action")
				return cfg
			}(),
			ctx:     context.Background(),
			wantErr: "unsupported action",
		},
		{
			name: "rolling step-down dispatches to leader lookup",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionRollingStepDownLeader
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find leader",
		},
		{
			name: "bluegreen join dispatches to blue leader lookup",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenJoinGreenNonVoters
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find Blue leader",
		},
		{
			name: "bluegreen sync dispatches to blue leader lookup",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenWaitGreenSynced
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find Blue leader",
		},
		{
			name: "bluegreen promote dispatches to blue leader lookup",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenPromoteGreenVoters
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find Blue leader",
		},
		{
			name: "bluegreen demote dispatches to initial leader lookup",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenDemoteBlueNonVotersStepDown
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find initial leader",
		},
		{
			name: "blue peers removal dispatches to generic remove flow",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenRemoveBluePeers
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find leader",
		},
		{
			name: "green peers removal dispatches to generic remove flow",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenRemoveGreenPeers
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find leader",
		},
		{
			name: "consensus repair dispatches to fallback leader lookup",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.Action = ExecutorActionBlueGreenRepairConsensus
				return cfg
			}(),
			ctx:     canceledCtx,
			wantErr: "failed to find leader for consensus repair",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := RunExecutor(tt.ctx, logr.Discard(), tt.cfg)
			if tt.wantOkay {
				if err != nil {
					t.Fatalf("RunExecutor() error=%v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("RunExecutor() error=nil, want contains %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("RunExecutor() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFindLeaderWithFallback(t *testing.T) {
	t.Parallel()

	ctx := canceledContext()
	cfg := baseExecutorTestConfig()

	tests := []struct {
		name             string
		primaryRevision  string
		fallbackRevision string
		primaryLabel     string
		fallbackLabel    string
		wantErr          string
	}{
		{
			name:             "fallback disabled when empty",
			primaryRevision:  "green",
			fallbackRevision: "",
			primaryLabel:     "Green",
			fallbackLabel:    "Blue",
			wantErr:          "failed to find leader among Green pods",
		},
		{
			name:             "fallback disabled when same revision",
			primaryRevision:  "green",
			fallbackRevision: "green",
			primaryLabel:     "Green",
			fallbackLabel:    "Blue",
			wantErr:          "failed to find leader among Green pods",
		},
		{
			name:             "fallback attempted and fails",
			primaryRevision:  "green",
			fallbackRevision: "blue",
			primaryLabel:     "Green",
			fallbackLabel:    "Blue",
			wantErr:          "failed to find leader (checked Green and Blue)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leaderURL, err := findLeaderWithFallback(
				ctx,
				logr.Discard(),
				cfg,
				tt.primaryRevision,
				tt.fallbackRevision,
				tt.primaryLabel,
				tt.fallbackLabel,
			)
			if err == nil {
				t.Fatalf("findLeaderWithFallback() err=nil, want contains %q", tt.wantErr)
			}
			if leaderURL != "" {
				t.Fatalf("findLeaderWithFallback() leaderURL=%q, want empty", leaderURL)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("findLeaderWithFallback() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFindLeaderWithFallbackUsing(t *testing.T) {
	t.Parallel()

	type findResult struct {
		url string
		err error
	}

	tests := []struct {
		name             string
		primaryRevision  string
		fallbackRevision string
		resultsByRev     map[string]findResult
		wantURL          string
		wantErr          string
		wantCalls        []string
	}{
		{
			name:             "primary success skips fallback",
			primaryRevision:  "green",
			fallbackRevision: "blue",
			resultsByRev: map[string]findResult{
				"green": {url: "https://green-leader"},
			},
			wantURL:   "https://green-leader",
			wantCalls: []string{"green"},
		},
		{
			name:             "primary failure with empty fallback",
			primaryRevision:  "green",
			fallbackRevision: "",
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no leader in green")},
			},
			wantErr:   "failed to find leader among Green pods",
			wantCalls: []string{"green"},
		},
		{
			name:             "primary failure then fallback success",
			primaryRevision:  "green",
			fallbackRevision: "blue",
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no leader in green")},
				"blue":  {url: "https://blue-leader"},
			},
			wantURL:   "https://blue-leader",
			wantCalls: []string{"green", "blue"},
		},
		{
			name:             "both revisions fail",
			primaryRevision:  "green",
			fallbackRevision: "blue",
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no leader in green")},
				"blue":  {err: errors.New("no leader in blue")},
			},
			wantErr:   "failed to find leader (checked Green and Blue)",
			wantCalls: []string{"green", "blue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := baseExecutorTestConfig()
			calls := make([]string, 0, 2)
			finder := func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
				calls = append(calls, revision)
				if result, ok := tt.resultsByRev[revision]; ok {
					return result.url, result.err
				}
				return "", errors.New("unexpected revision")
			}

			gotURL, err := findLeaderWithFallbackUsing(
				context.Background(),
				logr.Discard(),
				cfg,
				tt.primaryRevision,
				tt.fallbackRevision,
				"Green",
				"Blue",
				finder,
			)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("findLeaderWithFallbackUsing() unexpected error: %v", err)
				}
				if gotURL != tt.wantURL {
					t.Fatalf("findLeaderWithFallbackUsing() url=%q, want %q", gotURL, tt.wantURL)
				}
			} else {
				if err == nil {
					t.Fatalf("findLeaderWithFallbackUsing() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("findLeaderWithFallbackUsing() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
			}

			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("find calls=%v, want %v", calls, tt.wantCalls)
			}
			for i := range tt.wantCalls {
				if calls[i] != tt.wantCalls[i] {
					t.Fatalf("find call[%d]=%q, want %q", i, calls[i], tt.wantCalls[i])
				}
			}
		})
	}
}

func TestResolveLeaderWithPolicyUsing(t *testing.T) {
	t.Parallel()

	type findResult struct {
		url string
		err error
	}

	tests := []struct {
		name            string
		policy          leaderSearchPolicy
		resultsByRev    map[string]findResult
		wantValue       string
		wantDecision    string
		wantReason      string
		wantAttempts    int
		wantPrimaryErr  bool
		wantFallbackErr bool
	}{
		{
			name:         "primary success",
			policy:       newLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{"green": {url: "https://green-leader"}},
			wantValue:    "https://green-leader",
			wantDecision: decisionPathPrimarySuccess,
			wantReason:   reasonPrimaryLeaderFound,
			wantAttempts: 1,
		},
		{
			name:   "primary fail fallback success",
			policy: newLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {url: "https://blue-leader"},
			},
			wantValue:      "https://blue-leader",
			wantDecision:   decisionPathPrimaryFailedFallbackSuccess,
			wantReason:     reasonFallbackLeaderFound,
			wantAttempts:   2,
			wantPrimaryErr: true,
		},
		{
			name:   "both fail",
			policy: newLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {err: errors.New("no blue leader")},
			},
			wantDecision:    decisionPathPrimaryFailedFallbackFailed,
			wantReason:      reasonFallbackLeaderNotFound,
			wantAttempts:    2,
			wantPrimaryErr:  true,
			wantFallbackErr: true,
		},
		{
			name:   "fallback disabled",
			policy: newLeaderSearchPolicy("green", "", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
			},
			wantDecision:   decisionPathPrimaryFailedNoFallback,
			wantReason:     reasonPrimaryLeaderNotFound,
			wantAttempts:   1,
			wantPrimaryErr: true,
		},
		{
			name:   "context canceled classified deterministically",
			policy: newLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {err: context.Canceled},
			},
			wantDecision:    decisionPathContextCanceled,
			wantReason:      reasonContextCanceled,
			wantAttempts:    2,
			wantPrimaryErr:  true,
			wantFallbackErr: true,
		},
		{
			name:   "deadline exceeded classified deterministically",
			policy: newLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {err: context.DeadlineExceeded},
			},
			wantDecision:    decisionPathDeadlineExceeded,
			wantReason:      reasonDeadlineExceeded,
			wantAttempts:    2,
			wantPrimaryErr:  true,
			wantFallbackErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			finder := func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
				if result, ok := tt.resultsByRev[revision]; ok {
					return result.url, result.err
				}
				return "", errors.New("unexpected revision")
			}

			got := resolveLeaderWithPolicyUsing(context.Background(), baseExecutorTestConfig(), tt.policy, finder)

			if got.Value != tt.wantValue {
				t.Fatalf("value=%q, want %q", got.Value, tt.wantValue)
			}
			if got.DecisionPath != tt.wantDecision {
				t.Fatalf("decision=%q, want %q", got.DecisionPath, tt.wantDecision)
			}
			if got.ReasonCode != tt.wantReason {
				t.Fatalf("reason=%q, want %q", got.ReasonCode, tt.wantReason)
			}
			if got.AttemptsUsed != tt.wantAttempts {
				t.Fatalf("attempts=%d, want %d", got.AttemptsUsed, tt.wantAttempts)
			}
			if (got.PrimaryError != nil) != tt.wantPrimaryErr {
				t.Fatalf("primaryErr present=%v, want %v", got.PrimaryError != nil, tt.wantPrimaryErr)
			}
			if (got.FallbackError != nil) != tt.wantFallbackErr {
				t.Fatalf("fallbackErr present=%v, want %v", got.FallbackError != nil, tt.wantFallbackErr)
			}
		})
	}
}

func TestWaitForNewLeaderURLWithFuncs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		waitFn          func(context.Context, *ExecutorConfig, string) leaderElectionOutcome
		fallbackFn      func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error)
		wantURL         string
		wantErr         string
		wantReasonCode  string
		wantFallbackRun bool
	}{
		{
			name: "wait function returns new leader directly",
			waitFn: func(context.Context, *ExecutorConfig, string) leaderElectionOutcome {
				return leaderElectionOutcome{
					Value:        "https://green-leader",
					DecisionPath: decisionPathElectionObservedNewLeader,
					ReasonCode:   reasonElectionNewLeaderFound,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error) {
				return "", errors.New("fallback should not run")
			},
			wantURL:         "https://green-leader",
			wantFallbackRun: false,
		},
		{
			name: "deadline exceeded falls back to finder",
			waitFn: func(context.Context, *ExecutorConfig, string) leaderElectionOutcome {
				return leaderElectionOutcome{
					DecisionPath: decisionPathDeadlineExceeded,
					ReasonCode:   reasonDeadlineExceeded,
					WaitError:    context.DeadlineExceeded,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error) {
				return "https://fallback-leader", nil
			},
			wantURL:         "https://fallback-leader",
			wantFallbackRun: true,
		},
		{
			name: "same leader observation falls back to finder",
			waitFn: func(context.Context, *ExecutorConfig, string) leaderElectionOutcome {
				return leaderElectionOutcome{
					Value:        "https://previous-leader",
					DecisionPath: decisionPathElectionObservedSameLeader,
					ReasonCode:   reasonElectionSameLeaderSeen,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error) {
				return "https://fallback-leader", nil
			},
			wantURL:         "https://fallback-leader",
			wantFallbackRun: true,
		},
		{
			name: "unexpected wait error is returned",
			waitFn: func(context.Context, *ExecutorConfig, string) leaderElectionOutcome {
				return leaderElectionOutcome{
					DecisionPath: decisionPathElectionTimeout,
					ReasonCode:   reasonElectionTimeout,
					WaitError:    errors.New("wait exploded"),
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error) {
				return "https://fallback-leader", nil
			},
			wantErr:         "failed while waiting for new leader election",
			wantReasonCode:  reasonElectionTimeout,
			wantFallbackRun: false,
		},
		{
			name: "fallback failure is wrapped",
			waitFn: func(context.Context, *ExecutorConfig, string) leaderElectionOutcome {
				return leaderElectionOutcome{
					DecisionPath: decisionPathContextCanceled,
					ReasonCode:   reasonContextCanceled,
					WaitError:    context.Canceled,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string, string, string) (string, error) {
				return "", errors.New("could not find fallback leader")
			},
			wantErr:         "failed to find new leader after step-down",
			wantReasonCode:  reasonFallbackLeaderNotFound,
			wantFallbackRun: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := baseExecutorTestConfig()
			fallbackCalls := 0
			gotURL, err := waitForNewLeaderURLWithFuncs(
				context.Background(),
				logr.Discard(),
				cfg,
				"https://previous-leader",
				tt.waitFn,
				func(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, primaryRevision, fallbackRevision, primaryLabel, fallbackLabel string) (string, error) {
					fallbackCalls++
					return tt.fallbackFn(ctx, logger, cfg, primaryRevision, fallbackRevision, primaryLabel, fallbackLabel)
				},
			)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("waitForNewLeaderURLWithFuncs() unexpected error: %v", err)
				}
				if gotURL != tt.wantURL {
					t.Fatalf("waitForNewLeaderURLWithFuncs() url=%q, want %q", gotURL, tt.wantURL)
				}
			} else {
				if err == nil {
					t.Fatalf("waitForNewLeaderURLWithFuncs() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("waitForNewLeaderURLWithFuncs() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
				if tt.wantReasonCode != "" {
					if gotReason := reasonCodeFromError(err); gotReason != tt.wantReasonCode {
						t.Fatalf("waitForNewLeaderURLWithFuncs() reason=%q, want %q", gotReason, tt.wantReasonCode)
					}
				}
			}

			if tt.wantFallbackRun && fallbackCalls == 0 {
				t.Fatalf("expected fallback finder to be called")
			}
			if !tt.wantFallbackRun && fallbackCalls != 0 {
				t.Fatalf("expected fallback finder not to be called, got %d calls", fallbackCalls)
			}
		})
	}
}

func TestWaitForLeaderElectionWithFinderAndPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		ctx              context.Context
		previousLeader   string
		policy           retryPolicy
		finder           leaderOnceFinder
		wantDecisionPath string
		wantReasonCode   string
		wantValue        string
		wantWaitErr      bool
		wantErrIs        error
	}{
		{
			name:           "green leader observed",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: retryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    20 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == "green" {
					return "https://green-0", true
				}
				return "", false
			},
			wantDecisionPath: decisionPathElectionObservedNewLeader,
			wantReasonCode:   reasonElectionNewLeaderFound,
			wantValue:        "https://green-0",
		},
		{
			name:           "blue leader changed from previous",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: retryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    20 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == "blue" {
					return "https://blue-1", true
				}
				return "", false
			},
			wantDecisionPath: decisionPathElectionObservedNewLeader,
			wantReasonCode:   reasonElectionNewLeaderFound,
			wantValue:        "https://blue-1",
		},
		{
			name:           "same blue leader times out and is classified",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: retryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    5 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == "blue" {
					return "https://blue-0", true
				}
				return "", false
			},
			wantDecisionPath: decisionPathElectionObservedSameLeader,
			wantReasonCode:   reasonElectionSameLeaderSeen,
			wantValue:        "https://blue-0",
			wantWaitErr:      true,
			wantErrIs:        context.DeadlineExceeded,
		},
		{
			name:           "no leader observed and timed out",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: retryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    5 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, _ string) (string, bool) {
				return "", false
			},
			wantDecisionPath: decisionPathElectionTimeout,
			wantReasonCode:   reasonElectionTimeout,
			wantWaitErr:      true,
			wantErrIs:        context.DeadlineExceeded,
		},
		{
			name:           "context canceled is propagated deterministically",
			ctx:            canceledContext(),
			previousLeader: "https://blue-0",
			policy: retryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    20 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, _ string) (string, bool) {
				return "", false
			},
			wantDecisionPath: decisionPathContextCanceled,
			wantReasonCode:   reasonContextCanceled,
			wantWaitErr:      true,
			wantErrIs:        context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outcome := waitForLeaderElectionWithFinderAndPolicy(
				tt.ctx,
				baseExecutorTestConfig(),
				tt.previousLeader,
				tt.policy,
				tt.finder,
			)

			if outcome.DecisionPath != tt.wantDecisionPath {
				t.Fatalf("DecisionPath=%q, want %q", outcome.DecisionPath, tt.wantDecisionPath)
			}
			if outcome.ReasonCode != tt.wantReasonCode {
				t.Fatalf("ReasonCode=%q, want %q", outcome.ReasonCode, tt.wantReasonCode)
			}
			if outcome.Value != tt.wantValue {
				t.Fatalf("Value=%q, want %q", outcome.Value, tt.wantValue)
			}
			if (outcome.WaitError != nil) != tt.wantWaitErr {
				t.Fatalf("WaitError present=%v, want %v", outcome.WaitError != nil, tt.wantWaitErr)
			}
			if tt.wantErrIs != nil && !errors.Is(outcome.WaitError, tt.wantErrIs) {
				t.Fatalf("errors.Is(WaitError, %v)=false, want true (got: %v)", tt.wantErrIs, outcome.WaitError)
			}
		})
	}
}

func TestWaitForLeaderElection(t *testing.T) {
	t.Parallel()

	url, err := waitForLeaderElection(canceledContext(), baseExecutorTestConfig(), "previous-leader")
	if err == nil {
		t.Fatalf("waitForLeaderElection() error=nil, want context cancellation")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitForLeaderElection() error=%v, want context canceled/deadline exceeded", err)
	}
	if url != "" {
		t.Fatalf("waitForLeaderElection() url=%q, want empty", url)
	}
}

func TestWaitForNewLeaderURL(t *testing.T) {
	t.Parallel()

	cfg := baseExecutorTestConfig()
	newLeaderURL, err := waitForNewLeaderURL(canceledContext(), logr.Discard(), cfg, "previous-leader")
	if err == nil {
		t.Fatalf("waitForNewLeaderURL() error=nil, want failure")
	}
	if newLeaderURL != "" {
		t.Fatalf("waitForNewLeaderURL() newLeaderURL=%q, want empty", newLeaderURL)
	}
	if !strings.Contains(err.Error(), "failed to find new leader after step-down") {
		t.Fatalf("waitForNewLeaderURL() error=%q, want wrapped leader-finding failure", err.Error())
	}
}

func TestFindInitialLeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     context.Context
		cfg     *ExecutorConfig
		wantErr string
	}{
		{
			name:    "leader lookup failure is wrapped",
			ctx:     canceledContext(),
			cfg:     baseExecutorTestConfig(),
			wantErr: "failed to find initial leader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			leaderURL, err := findInitialLeader(tt.ctx, logr.Discard(), tt.cfg)
			if err == nil {
				t.Fatalf("findInitialLeader() error=nil, want contains %q", tt.wantErr)
			}
			if leaderURL != "" {
				t.Fatalf("findInitialLeader() leaderURL=%q, want empty", leaderURL)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("findInitialLeader() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunBlueGreenRepairConsensusValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *ExecutorConfig
		wantErr string
	}{
		{
			name: "missing blue revision",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.BlueRevision = ""
				return cfg
			}(),
			wantErr: "blue revision is required for consensus repair",
		},
		{
			name: "missing green revision",
			cfg: func() *ExecutorConfig {
				cfg := baseExecutorTestConfig()
				cfg.GreenRevision = ""
				return cfg
			}(),
			wantErr: "green revision is required for consensus repair",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := runBlueGreenRepairConsensus(context.Background(), logr.Discard(), tt.cfg)
			if err == nil {
				t.Fatalf("runBlueGreenRepairConsensus() error=nil, want contains %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("runBlueGreenRepairConsensus() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunBlueGreenRemovePeersValidation(t *testing.T) {
	t.Parallel()

	err := runBlueGreenRemovePeers(
		context.Background(),
		logr.Discard(),
		baseExecutorTestConfig(),
		"",
		"green",
		"blue",
		"Blue",
	)
	if err == nil {
		t.Fatalf("runBlueGreenRemovePeers() error=nil, want validation error")
	}
	if !strings.Contains(err.Error(), "revision to remove is required") {
		t.Fatalf("runBlueGreenRemovePeers() error=%q, want revision validation error", err.Error())
	}
}

func TestFindLeaderAndFindLeaderOnceValidation(t *testing.T) {
	t.Parallel()

	leaderURL, ok := findLeaderOnce(context.Background(), nil, "")
	if ok {
		t.Fatalf("findLeaderOnce() ok=true, want false")
	}
	if leaderURL != "" {
		t.Fatalf("findLeaderOnce() leaderURL=%q, want empty", leaderURL)
	}

	_, err := findLeader(context.Background(), nil, "")
	if err == nil {
		t.Fatalf("findLeader() error=nil, want config validation error")
	}
	if !strings.Contains(err.Error(), "config is required") {
		t.Fatalf("findLeader() error=%q, want contains %q", err.Error(), "config is required")
	}
}

func TestRaftLeaderInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     *openbao.RaftConfigurationResponse
		bluePrefix string
		wantID     string
		wantBlue   bool
	}{
		{
			name:       "nil config",
			config:     nil,
			bluePrefix: "openbao-blue-",
			wantID:     "",
			wantBlue:   false,
		},
		{
			name: "no leader in config",
			config: &openbao.RaftConfigurationResponse{
				Config: openbao.RaftConfiguration{
					Servers: []openbao.RaftServer{
						{NodeID: "openbao-blue-0", Leader: false},
					},
				},
			},
			bluePrefix: "openbao-blue-",
			wantID:     "",
			wantBlue:   false,
		},
		{
			name: "leader is blue by node id",
			config: &openbao.RaftConfigurationResponse{
				Config: openbao.RaftConfiguration{
					Servers: []openbao.RaftServer{
						{NodeID: "openbao-blue-1", Leader: true},
					},
				},
			},
			bluePrefix: "openbao-blue-",
			wantID:     "openbao-blue-1",
			wantBlue:   true,
		},
		{
			name: "leader is blue by address",
			config: &openbao.RaftConfigurationResponse{
				Config: openbao.RaftConfiguration{
					Servers: []openbao.RaftServer{
						{
							NodeID:  "node-1",
							Address: "https://openbao-blue-2.openbao.default.svc:8201",
							Leader:  true,
						},
					},
				},
			},
			bluePrefix: "openbao-blue-",
			wantID:     "node-1",
			wantBlue:   true,
		},
		{
			name: "leader is not blue",
			config: &openbao.RaftConfigurationResponse{
				Config: openbao.RaftConfiguration{
					Servers: []openbao.RaftServer{
						{NodeID: "openbao-green-0", Leader: true},
					},
				},
			},
			bluePrefix: "openbao-blue-",
			wantID:     "openbao-green-0",
			wantBlue:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotBlue := raftLeaderInfo(tt.config, tt.bluePrefix)
			if gotID != tt.wantID {
				t.Fatalf("raftLeaderInfo() id=%q, want %q", gotID, tt.wantID)
			}
			if gotBlue != tt.wantBlue {
				t.Fatalf("raftLeaderInfo() isBlue=%v, want %v", gotBlue, tt.wantBlue)
			}
		})
	}
}

func baseExecutorTestConfig() *ExecutorConfig {
	return &ExecutorConfig{
		ClusterNamespace: "default",
		ClusterName:      "openbao",
		ClusterReplicas:  0,
		Action:           ExecutorActionRollingStepDownLeader,
		JWTAuthRole:      "upgrade-role",
		JWTToken:         "jwt-token",
		TLSCACert:        []byte("ca-data"),
		BlueRevision:     "blue",
		GreenRevision:    "green",
		SyncThreshold:    100,
		Timeout:          2 * time.Second,
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
