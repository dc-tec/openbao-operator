package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
	"github.com/go-logr/logr"
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

func TestFindLeaderWithPolicy(t *testing.T) {
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

			leaderURL, err := raftops.FindPreferredLeaderWithFallback(
				ctx,
				logr.Discard(),
				cfg,
				tt.primaryRevision,
				tt.fallbackRevision,
				tt.primaryLabel,
				tt.fallbackLabel,
			)
			if err == nil {
				t.Fatalf("raftops.FindPreferredLeaderWithFallback() err=nil, want contains %q", tt.wantErr)
			}
			if leaderURL != "" {
				t.Fatalf("raftops.FindPreferredLeaderWithFallback() leaderURL=%q, want empty", leaderURL)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("raftops.FindPreferredLeaderWithFallback() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFindLeaderWithPolicyUsingFallbackBehavior(t *testing.T) {
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

			policy := raftops.NewLeaderSearchPolicy(tt.primaryRevision, tt.fallbackRevision, "Green", "Blue")

			gotURL, err := raftops.FindLeaderWithPolicyUsing(
				context.Background(),
				logr.Discard(),
				cfg,
				policy,
				finder,
			)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("raftops.FindLeaderWithPolicyUsing() unexpected error: %v", err)
				}
				if gotURL != tt.wantURL {
					t.Fatalf("raftops.FindLeaderWithPolicyUsing() url=%q, want %q", gotURL, tt.wantURL)
				}
			} else {
				if err == nil {
					t.Fatalf("raftops.FindLeaderWithPolicyUsing() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("raftops.FindLeaderWithPolicyUsing() error=%q, want contains %q", err.Error(), tt.wantErr)
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

func TestFindLeaderWithPolicyUsing(t *testing.T) {
	t.Parallel()

	const (
		greenRevision = "green"
		blueRevision  = "blue"
	)

	policy := raftops.NewLeaderSearchPolicy("green", "blue", "Green", "Blue")
	calls := make([]string, 0, 2)
	finder := func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
		calls = append(calls, revision)
		if revision == greenRevision {
			return "", errors.New("no leader in green")
		}
		if revision == blueRevision {
			return "https://blue-leader", nil
		}
		return "", errors.New("unexpected revision")
	}

	gotURL, err := raftops.FindLeaderWithPolicyUsing(
		context.Background(),
		logr.Discard(),
		baseExecutorTestConfig(),
		policy,
		finder,
	)
	if err != nil {
		t.Fatalf("raftops.FindLeaderWithPolicyUsing() unexpected error: %v", err)
	}
	if gotURL != "https://blue-leader" {
		t.Fatalf("raftops.FindLeaderWithPolicyUsing() url=%q, want %q", gotURL, "https://blue-leader")
	}
	if len(calls) != 2 {
		t.Fatalf("find calls=%v, want [green blue]", calls)
	}
	if calls[0] != greenRevision || calls[1] != blueRevision {
		t.Fatalf("find calls=%v, want [green blue]", calls)
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
		policy          raftops.LeaderSearchPolicy
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
			policy:       raftops.NewLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{"green": {url: "https://green-leader"}},
			wantValue:    "https://green-leader",
			wantDecision: raftops.DecisionPathPrimarySuccess,
			wantReason:   raftops.ReasonPrimaryLeaderFound,
			wantAttempts: 1,
		},
		{
			name:   "primary fail fallback success",
			policy: raftops.NewLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {url: "https://blue-leader"},
			},
			wantValue:      "https://blue-leader",
			wantDecision:   raftops.DecisionPathPrimaryFailedFallbackSuccess,
			wantReason:     raftops.ReasonFallbackLeaderFound,
			wantAttempts:   2,
			wantPrimaryErr: true,
		},
		{
			name:   "both fail",
			policy: raftops.NewLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {err: errors.New("no blue leader")},
			},
			wantDecision:    raftops.DecisionPathPrimaryFailedFallbackFailed,
			wantReason:      raftops.ReasonFallbackLeaderNotFound,
			wantAttempts:    2,
			wantPrimaryErr:  true,
			wantFallbackErr: true,
		},
		{
			name:   "fallback disabled",
			policy: raftops.NewLeaderSearchPolicy("green", "", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
			},
			wantDecision:   raftops.DecisionPathPrimaryFailedNoFallback,
			wantReason:     raftops.ReasonPrimaryLeaderNotFound,
			wantAttempts:   1,
			wantPrimaryErr: true,
		},
		{
			name:   "context canceled classified deterministically",
			policy: raftops.NewLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {err: context.Canceled},
			},
			wantDecision:    raftops.DecisionPathContextCanceled,
			wantReason:      raftops.ReasonContextCanceled,
			wantAttempts:    2,
			wantPrimaryErr:  true,
			wantFallbackErr: true,
		},
		{
			name:   "deadline exceeded classified deterministically",
			policy: raftops.NewLeaderSearchPolicy("green", "blue", "Green", "Blue"),
			resultsByRev: map[string]findResult{
				"green": {err: errors.New("no green leader")},
				"blue":  {err: context.DeadlineExceeded},
			},
			wantDecision:    raftops.DecisionPathDeadlineExceeded,
			wantReason:      raftops.ReasonDeadlineExceeded,
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

			got := raftops.ResolveLeaderWithPolicyUsing(context.Background(), baseExecutorTestConfig(), tt.policy, finder)

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

	const fallbackLeaderURL = "https://fallback-leader"

	tests := []struct {
		name            string
		ctx             context.Context
		waitFn          func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome
		fallbackFn      func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error)
		wantURL         string
		wantErr         string
		wantReasonCode  string
		wantFallbackRun bool
	}{
		{
			name: "wait function returns new leader directly",
			ctx:  context.Background(),
			waitFn: func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome {
				return raftops.LeaderElectionOutcome{
					Value:        "https://green-leader",
					DecisionPath: raftops.DecisionPathElectionObservedNewLeader,
					ReasonCode:   raftops.ReasonElectionNewLeaderFound,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return "", errors.New("fallback should not run")
			},
			wantURL:         "https://green-leader",
			wantFallbackRun: false,
		},
		{
			name: "deadline exceeded falls back to finder",
			ctx:  context.Background(),
			waitFn: func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome {
				return raftops.LeaderElectionOutcome{
					DecisionPath: raftops.DecisionPathDeadlineExceeded,
					ReasonCode:   raftops.ReasonDeadlineExceeded,
					WaitError:    context.DeadlineExceeded,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return fallbackLeaderURL, nil
			},
			wantURL:         fallbackLeaderURL,
			wantFallbackRun: true,
		},
		{
			name: "same leader observation returns observed leader without fallback",
			ctx:  context.Background(),
			waitFn: func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome {
				return raftops.LeaderElectionOutcome{
					Value:        "https://previous-leader",
					DecisionPath: raftops.DecisionPathElectionObservedSameLeader,
					ReasonCode:   raftops.ReasonElectionSameLeaderSeen,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return fallbackLeaderURL, nil
			},
			wantURL:         "https://previous-leader",
			wantFallbackRun: false,
		},
		{
			name: "unexpected wait error is returned",
			ctx:  context.Background(),
			waitFn: func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome {
				return raftops.LeaderElectionOutcome{
					DecisionPath: raftops.DecisionPathElectionTimeout,
					ReasonCode:   raftops.ReasonElectionTimeout,
					WaitError:    errors.New("wait exploded"),
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return fallbackLeaderURL, nil
			},
			wantErr:         "failed while waiting for new leader election",
			wantReasonCode:  raftops.ReasonElectionTimeout,
			wantFallbackRun: false,
		},
		{
			name: "fallback failure is wrapped when context is active",
			ctx:  context.Background(),
			waitFn: func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome {
				return raftops.LeaderElectionOutcome{
					DecisionPath: raftops.DecisionPathContextCanceled,
					ReasonCode:   raftops.ReasonContextCanceled,
					WaitError:    context.Canceled,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return "", errors.New("could not find fallback leader")
			},
			wantErr:         "failed to find new leader after step-down",
			wantReasonCode:  raftops.ReasonFallbackLeaderNotFound,
			wantFallbackRun: true,
		},
		{
			name: "parent context cancellation fails fast without fallback",
			ctx:  canceledContext(),
			waitFn: func(context.Context, *ExecutorConfig, string) raftops.LeaderElectionOutcome {
				return raftops.LeaderElectionOutcome{
					DecisionPath: raftops.DecisionPathContextCanceled,
					ReasonCode:   raftops.ReasonContextCanceled,
					WaitError:    context.Canceled,
				}
			},
			fallbackFn: func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return fallbackLeaderURL, nil
			},
			wantErr:         "failed while waiting for new leader election",
			wantReasonCode:  raftops.ReasonContextCanceled,
			wantFallbackRun: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := baseExecutorTestConfig()
			fallbackCalls := 0
			testCtx := tt.ctx
			if testCtx == nil {
				testCtx = context.Background()
			}
			gotURL, err := raftops.WaitForNewLeaderURLWithFuncs(
				testCtx,
				logr.Discard(),
				cfg,
				"https://previous-leader",
				tt.waitFn,
				func(ctx context.Context, logger logr.Logger, cfg *ExecutorConfig, firstRevision string, secondRevision string) (string, error) {
					fallbackCalls++
					return tt.fallbackFn(ctx, logger, cfg, firstRevision, secondRevision)
				},
			)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("raftops.WaitForNewLeaderURLWithFuncs() unexpected error: %v", err)
				}
				if gotURL != tt.wantURL {
					t.Fatalf("raftops.WaitForNewLeaderURLWithFuncs() url=%q, want %q", gotURL, tt.wantURL)
				}
			} else {
				if err == nil {
					t.Fatalf("raftops.WaitForNewLeaderURLWithFuncs() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("raftops.WaitForNewLeaderURLWithFuncs() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
				if tt.wantReasonCode != "" {
					if gotReason := raftops.ReasonCodeFromError(err); gotReason != tt.wantReasonCode {
						t.Fatalf("raftops.WaitForNewLeaderURLWithFuncs() reason=%q, want %q", gotReason, tt.wantReasonCode)
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
		policy           raftops.RetryPolicy
		finder           raftops.LeaderOnceFinder
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
			policy: raftops.RetryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    20 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == "green" {
					return "https://green-0", true
				}
				return "", false
			},
			wantDecisionPath: raftops.DecisionPathElectionObservedNewLeader,
			wantReasonCode:   raftops.ReasonElectionNewLeaderFound,
			wantValue:        "https://green-0",
		},
		{
			name:           "blue leader changed from previous",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: raftops.RetryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    20 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == "blue" {
					return "https://blue-1", true
				}
				return "", false
			},
			wantDecisionPath: raftops.DecisionPathElectionObservedNewLeader,
			wantReasonCode:   raftops.ReasonElectionNewLeaderFound,
			wantValue:        "https://blue-1",
		},
		{
			name:           "same blue leader times out and is classified",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: raftops.RetryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    5 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == "blue" {
					return "https://blue-0", true
				}
				return "", false
			},
			wantDecisionPath: raftops.DecisionPathElectionObservedSameLeader,
			wantReasonCode:   raftops.ReasonElectionSameLeaderSeen,
			wantValue:        "https://blue-0",
			wantWaitErr:      true,
			wantErrIs:        context.DeadlineExceeded,
		},
		{
			name:           "no leader observed and timed out",
			ctx:            context.Background(),
			previousLeader: "https://blue-0",
			policy: raftops.RetryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    5 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, _ string) (string, bool) {
				return "", false
			},
			wantDecisionPath: raftops.DecisionPathElectionTimeout,
			wantReasonCode:   raftops.ReasonElectionTimeout,
			wantWaitErr:      true,
			wantErrIs:        context.DeadlineExceeded,
		},
		{
			name:           "context canceled is propagated deterministically",
			ctx:            canceledContext(),
			previousLeader: "https://blue-0",
			policy: raftops.RetryPolicy{
				AttemptInterval: time.Millisecond,
				ElectionWait:    20 * time.Millisecond,
			},
			finder: func(_ context.Context, _ *ExecutorConfig, _ string) (string, bool) {
				return "", false
			},
			wantDecisionPath: raftops.DecisionPathContextCanceled,
			wantReasonCode:   raftops.ReasonContextCanceled,
			wantWaitErr:      true,
			wantErrIs:        context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outcome := raftops.WaitForLeaderElectionWithFinderAndPolicy(
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

func TestWaitForLeaderElectionOutcome(t *testing.T) {
	t.Parallel()

	outcome := raftops.WaitForLeaderElectionOutcome(canceledContext(), baseExecutorTestConfig(), "previous-leader")
	if outcome.WaitError == nil {
		t.Fatalf("raftops.WaitForLeaderElectionOutcome() WaitError=nil, want context cancellation")
	}
	if !errors.Is(outcome.WaitError, context.Canceled) && !errors.Is(outcome.WaitError, context.DeadlineExceeded) {
		t.Fatalf("raftops.WaitForLeaderElectionOutcome() error=%v, want context canceled/deadline exceeded", outcome.WaitError)
	}
	if outcome.Value != "" {
		t.Fatalf("raftops.WaitForLeaderElectionOutcome() value=%q, want empty", outcome.Value)
	}
}

func TestWaitForNewLeaderURL(t *testing.T) {
	t.Parallel()

	cfg := baseExecutorTestConfig()
	newLeaderURL, err := raftops.WaitForNewLeaderURL(canceledContext(), logr.Discard(), cfg, "previous-leader")
	if err == nil {
		t.Fatalf("raftops.WaitForNewLeaderURL() error=nil, want failure")
	}
	if newLeaderURL != "" {
		t.Fatalf("raftops.WaitForNewLeaderURL() newLeaderURL=%q, want empty", newLeaderURL)
	}
	if !strings.Contains(err.Error(), "failed while waiting for new leader election") {
		t.Fatalf("raftops.WaitForNewLeaderURL() error=%q, want wait interruption failure", err.Error())
	}
	if gotReason := raftops.ReasonCodeFromError(err); gotReason != raftops.ReasonContextCanceled {
		t.Fatalf("raftops.WaitForNewLeaderURL() reason=%q, want %q", gotReason, raftops.ReasonContextCanceled)
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

			leaderURL, err := raftops.FindInitialLeader(tt.ctx, logr.Discard(), tt.cfg)
			if err == nil {
				t.Fatalf("raftops.FindInitialLeader() error=nil, want contains %q", tt.wantErr)
			}
			if leaderURL != "" {
				t.Fatalf("raftops.FindInitialLeader() leaderURL=%q, want empty", leaderURL)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("raftops.FindInitialLeader() error=%q, want contains %q", err.Error(), tt.wantErr)
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
			err := raftops.RunBlueGreenRepairConsensus(context.Background(), logr.Discard(), tt.cfg)
			if err == nil {
				t.Fatalf("RunBlueGreenRepairConsensus() error=nil, want contains %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("RunBlueGreenRepairConsensus() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunBlueGreenRemovePeersAcceptsUnrevisionedTarget(t *testing.T) {
	t.Parallel()

	err := raftops.RunBlueGreenRemovePeers(
		canceledContext(),
		logr.Discard(),
		baseExecutorTestConfig(),
		"",
		"green",
		"",
		"Blue",
	)
	if err == nil {
		t.Fatal("RunBlueGreenRemovePeers() error=nil, want leader lookup error")
	}
	if strings.Contains(err.Error(), "revision to remove is required") {
		t.Fatalf("RunBlueGreenRemovePeers() rejected an unrevisioned target: %v", err)
	}
	if !strings.Contains(err.Error(), "failed to find leader") {
		t.Fatalf("RunBlueGreenRemovePeers() error=%q, want leader lookup error", err.Error())
	}
}

func TestFindLeaderAndFindLeaderOnceValidation(t *testing.T) {
	t.Parallel()

	leaderURL, ok := raftops.FindLeaderOnce(context.Background(), nil, "")
	if ok {
		t.Fatalf("raftops.FindLeaderOnce() ok=true, want false")
	}
	if leaderURL != "" {
		t.Fatalf("raftops.FindLeaderOnce() leaderURL=%q, want empty", leaderURL)
	}

	_, err := raftops.FindLeader(context.Background(), nil, "")
	if err == nil {
		t.Fatalf("raftops.FindLeader() error=nil, want config validation error")
	}
	if !strings.Contains(err.Error(), "config is required") {
		t.Fatalf("raftops.FindLeader() error=%q, want contains %q", err.Error(), "config is required")
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
