package raftops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	testBlueRevision    = "blue"
	testGreenRevision   = "green"
	testGreenLeaderURL0 = "https://vault-green-0"
	testGreenLeaderURL1 = "https://vault-green-1"
)

type demoterStub struct {
	calls []string
	errs  map[string]error
}

func (d *demoterStub) DemoteRaftPeer(_ context.Context, serverID string) error {
	d.calls = append(d.calls, serverID)
	if d.errs == nil {
		return nil
	}
	return d.errs[serverID]
}

func TestDemoteBlueVotersExceptLeader(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		BlueRevision:    testBlueRevision,
		ClusterReplicas: 3,
	}
	config := &portopenbao.RaftConfigurationResponse{
		Config: portopenbao.RaftConfiguration{
			Servers: []portopenbao.RaftServer{
				{NodeID: "vault-blue-0", Address: "vault-blue-0.vault.default.svc", Leader: true, Voter: true},
				{NodeID: "vault-blue-1", Address: "vault-blue-1.vault.default.svc", Voter: true},
				{NodeID: "vault-blue-2", Address: "vault-blue-2.vault.default.svc", Voter: false},
				{NodeID: "vault-green-0", Address: "vault-green-0.vault.default.svc", Voter: true},
			},
		},
	}

	t.Run("demotes only non leader blue voters", func(t *testing.T) {
		t.Parallel()

		demoter := &demoterStub{}
		err := DemoteBlueVotersExceptLeader(context.Background(), logr.Discard(), cfg, demoter, config, "vault-blue-0", "vault-blue-")
		if err != nil {
			t.Fatalf("DemoteBlueVotersExceptLeader() error = %v, want nil", err)
		}
		if len(demoter.calls) != 1 || demoter.calls[0] != "vault-blue-1" {
			t.Fatalf("DemoteRaftPeer calls = %v, want [vault-blue-1]", demoter.calls)
		}
	})

	t.Run("retryable demote errors are tolerated", func(t *testing.T) {
		t.Parallel()

		demoter := &demoterStub{
			errs: map[string]error{
				"vault-blue-1": errors.New("temporary demote failure"),
			},
		}
		err := DemoteBlueVotersExceptLeader(context.Background(), logr.Discard(), cfg, demoter, config, "vault-blue-0", "vault-blue-")
		if err != nil {
			t.Fatalf("DemoteBlueVotersExceptLeader() error = %v, want nil", err)
		}
	})

	t.Run("fatal demote errors stop the transfer biasing", func(t *testing.T) {
		t.Parallel()

		demoter := &demoterStub{
			errs: map[string]error{
				"vault-blue-1": errors.New("forbidden"),
			},
		}
		err := DemoteBlueVotersExceptLeader(context.Background(), logr.Discard(), cfg, demoter, config, "vault-blue-0", "vault-blue-")
		if err == nil {
			t.Fatalf("DemoteBlueVotersExceptLeader() error = nil, want fatal error")
		}
		if got := ReasonCodeFromError(err); got != ReasonDemoteFatal {
			t.Fatalf("ReasonCodeFromError() = %q, want %q", got, ReasonDemoteFatal)
		}
	})
}

func TestWaitForLeaderElectionWithFinderAndPolicy(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		BlueRevision:  testBlueRevision,
		GreenRevision: testGreenRevision,
	}

	t.Run("observes new green leader", func(t *testing.T) {
		t.Parallel()

		outcome := WaitForLeaderElectionWithFinderAndPolicy(
			context.Background(),
			cfg,
			"https://vault-blue-0",
			RetryPolicy{AttemptInterval: time.Millisecond, ElectionWait: 10 * time.Millisecond},
			func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == testGreenRevision {
					return testGreenLeaderURL0, true
				}
				return "", false
			},
		)

		if outcome.DecisionPath != DecisionPathElectionObservedNewLeader {
			t.Fatalf("DecisionPath = %q, want %q", outcome.DecisionPath, DecisionPathElectionObservedNewLeader)
		}
		if outcome.ReasonCode != ReasonElectionNewLeaderFound {
			t.Fatalf("ReasonCode = %q, want %q", outcome.ReasonCode, ReasonElectionNewLeaderFound)
		}
		if outcome.Value != testGreenLeaderURL0 {
			t.Fatalf("Value = %q, want green leader URL", outcome.Value)
		}
	})

	t.Run("timeout with same prior leader is captured", func(t *testing.T) {
		t.Parallel()

		outcome := WaitForLeaderElectionWithFinderAndPolicy(
			context.Background(),
			cfg,
			"https://vault-blue-0",
			RetryPolicy{AttemptInterval: time.Millisecond, ElectionWait: 3 * time.Millisecond},
			func(_ context.Context, _ *ExecutorConfig, revision string) (string, bool) {
				if revision == testBlueRevision {
					return "https://vault-blue-0", true
				}
				return "", false
			},
		)

		if outcome.DecisionPath != DecisionPathElectionObservedSameLeader {
			t.Fatalf("DecisionPath = %q, want %q", outcome.DecisionPath, DecisionPathElectionObservedSameLeader)
		}
		if outcome.ReasonCode != ReasonElectionSameLeaderSeen {
			t.Fatalf("ReasonCode = %q, want %q", outcome.ReasonCode, ReasonElectionSameLeaderSeen)
		}
		if outcome.WaitError == nil {
			t.Fatalf("WaitError = nil, want timeout error")
		}
	})
}

func TestWaitForNewLeaderURLWithFuncs(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		BlueRevision:  testBlueRevision,
		GreenRevision: testGreenRevision,
	}

	t.Run("returns observed leader without fallback", func(t *testing.T) {
		t.Parallel()

		leaderURL, err := WaitForNewLeaderURLWithFuncs(
			context.Background(),
			logr.Discard(),
			cfg,
			"https://vault-blue-0",
			func(context.Context, *ExecutorConfig, string) LeaderElectionOutcome {
				return LeaderElectionOutcome{
					Value:        testGreenLeaderURL0,
					DecisionPath: DecisionPathElectionObservedNewLeader,
					ReasonCode:   ReasonElectionNewLeaderFound,
				}
			},
			func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return "", errors.New("unexpected fallback call")
			},
		)
		if err != nil {
			t.Fatalf("WaitForNewLeaderURLWithFuncs() error = %v, want nil", err)
		}
		if leaderURL != testGreenLeaderURL0 {
			t.Fatalf("leaderURL = %q, want green leader URL", leaderURL)
		}
	})

	t.Run("falls back after election timeout", func(t *testing.T) {
		t.Parallel()

		leaderURL, err := WaitForNewLeaderURLWithFuncs(
			context.Background(),
			logr.Discard(),
			cfg,
			"https://vault-blue-0",
			func(context.Context, *ExecutorConfig, string) LeaderElectionOutcome {
				return LeaderElectionOutcome{
					DecisionPath: DecisionPathElectionTimeout,
					ReasonCode:   ReasonElectionTimeout,
					WaitError:    NewExecutorReasonedError(ReasonElectionTimeout, "timed out", context.DeadlineExceeded),
				}
			},
			func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return testGreenLeaderURL1, nil
			},
		)
		if err != nil {
			t.Fatalf("WaitForNewLeaderURLWithFuncs() error = %v, want nil", err)
		}
		if leaderURL != testGreenLeaderURL1 {
			t.Fatalf("leaderURL = %q, want fallback leader URL", leaderURL)
		}
	})

	t.Run("non timeout wait errors return immediately", func(t *testing.T) {
		t.Parallel()

		_, err := WaitForNewLeaderURLWithFuncs(
			context.Background(),
			logr.Discard(),
			cfg,
			"https://vault-blue-0",
			func(context.Context, *ExecutorConfig, string) LeaderElectionOutcome {
				return LeaderElectionOutcome{
					DecisionPath: DecisionPathPrimaryFailedFallbackFailed,
					ReasonCode:   ReasonLeaderTransferStateFailed,
					WaitError:    NewExecutorReasonedError(ReasonLeaderTransferStateFailed, "leader election failed", errors.New("boom")),
				}
			},
			func(context.Context, logr.Logger, *ExecutorConfig, string, string) (string, error) {
				return testGreenLeaderURL1, nil
			},
		)
		if err == nil {
			t.Fatalf("WaitForNewLeaderURLWithFuncs() error = nil, want failure")
		}
		if got := ReasonCodeFromError(err); got != ReasonLeaderTransferStateFailed {
			t.Fatalf("ReasonCodeFromError() = %q, want %q", got, ReasonLeaderTransferStateFailed)
		}
	})
}
