package bluegreen

import (
	"testing"
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestExecutorRunID(t *testing.T) {
	tests := []struct {
		name             string
		autoRollback     autoRollbackConfig
		jobFailureCount  int32
		expectedExecutor string
	}{
		{
			name:             "disabled",
			autoRollback:     autoRollbackConfig{Enabled: false, OnJobFailure: true},
			jobFailureCount:  1,
			expectedExecutor: "",
		},
		{
			name:             "onJobFailure disabled",
			autoRollback:     autoRollbackConfig{Enabled: true, OnJobFailure: false},
			jobFailureCount:  2,
			expectedExecutor: "",
		},
		{
			name:             "attempt 0 uses legacy empty runID",
			autoRollback:     autoRollbackConfig{Enabled: true, OnJobFailure: true},
			jobFailureCount:  0,
			expectedExecutor: "",
		},
		{
			name:             "attempt 1 uses retry-1",
			autoRollback:     autoRollbackConfig{Enabled: true, OnJobFailure: true},
			jobFailureCount:  1,
			expectedExecutor: "retry-1",
		},
		{
			name:             "attempt 2 uses retry-2",
			autoRollback:     autoRollbackConfig{Enabled: true, OnJobFailure: true},
			jobFailureCount:  2,
			expectedExecutor: "retry-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := executorRunID(tt.autoRollback, tt.jobFailureCount); got != tt.expectedExecutor {
				t.Fatalf("executorRunID() = %q, expected %q", got, tt.expectedExecutor)
			}
		})
	}
}

func TestExecutorJobDecision(t *testing.T) {
	tests := []struct {
		name              string
		autoRollback      autoRollbackConfig
		currentFailures   int32
		maxFailures       int32
		result            *JobResult
		expectedCompleted bool
		expectedKind      phaseOutcomeKind
		expectedFailures  int32
	}{
		{
			name:              "running requeues without changing failure count",
			autoRollback:      autoRollbackConfig{Enabled: true, OnJobFailure: true},
			currentFailures:   0,
			maxFailures:       2,
			result:            &JobResult{Name: "job", Exists: true, Running: true},
			expectedCompleted: false,
			expectedKind:      phaseOutcomeRequeueAfter,
			expectedFailures:  0,
		},
		{
			name:              "success completes and resets failure tracking",
			autoRollback:      autoRollbackConfig{Enabled: true, OnJobFailure: true},
			currentFailures:   1,
			maxFailures:       2,
			result:            &JobResult{Name: "job", Exists: true, Succeeded: true},
			expectedCompleted: true,
			expectedKind:      "",
			expectedFailures:  0,
		},
		{
			name:              "failed retries when enabled and under max",
			autoRollback:      autoRollbackConfig{Enabled: true, OnJobFailure: true},
			currentFailures:   0,
			maxFailures:       2,
			result:            &JobResult{Name: "job", Exists: true, Failed: true},
			expectedCompleted: false,
			expectedKind:      phaseOutcomeRequeueAfter,
			expectedFailures:  1,
		},
		{
			name:              "failed triggers rollback when max failures reached",
			autoRollback:      autoRollbackConfig{Enabled: true, OnJobFailure: true},
			currentFailures:   1,
			maxFailures:       2,
			result:            &JobResult{Name: "job", Exists: true, Failed: true},
			expectedCompleted: false,
			expectedKind:      phaseOutcomeRollback,
			expectedFailures:  2,
		},
		{
			name:              "failed holds when autoRollback disabled",
			autoRollback:      autoRollbackConfig{Enabled: false, OnJobFailure: true},
			currentFailures:   0,
			maxFailures:       2,
			result:            &JobResult{Name: "job", Exists: true, Failed: true},
			expectedCompleted: false,
			expectedKind:      phaseOutcomeHold,
			expectedFailures:  1,
		},
		{
			name:              "failed holds and does not increment past 1 when autoRollback disabled",
			autoRollback:      autoRollbackConfig{Enabled: false, OnJobFailure: true},
			currentFailures:   1,
			maxFailures:       2,
			result:            &JobResult{Name: "job", Exists: true, Failed: true},
			expectedCompleted: false,
			expectedKind:      phaseOutcomeHold,
			expectedFailures:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := executorJobDecision(tt.autoRollback, tt.currentFailures, tt.maxFailures, tt.result, "abort msg")
			if err != nil {
				t.Fatalf("executorJobDecision() error: %v", err)
			}
			if got.Completed != tt.expectedCompleted {
				t.Fatalf("Completed=%v, expected %v", got.Completed, tt.expectedCompleted)
			}
			if got.Outcome.kind != tt.expectedKind {
				t.Fatalf("Outcome.kind=%s, expected %s", got.Outcome.kind, tt.expectedKind)
			}
			if got.NextFailureCount != tt.expectedFailures {
				t.Fatalf("NextFailureCount=%d, expected %d", got.NextFailureCount, tt.expectedFailures)
			}
		})
	}
}

func TestDemotionPreconditionsSatisfied(t *testing.T) {
	tests := []struct {
		name        string
		pods        []podSnapshot
		desiredPods int
		strategy    openbaov1alpha1.BlueGreenTrafficStrategy
		trafficStep int32
		expectedOK  bool
	}{
		{
			name: "blocks when not fully ready+unsealed",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: false},
				{Ready: true, Unsealed: true},
			},
			desiredPods: 3,
			strategy:    openbaov1alpha1.BlueGreenTrafficStrategyServiceSelectors,
			expectedOK:  false,
		},
		{
			name: "blocks when gateway weights not at final step",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: true},
			},
			desiredPods: 3,
			strategy:    openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights,
			trafficStep: 2,
			expectedOK:  false,
		},
		{
			name: "allows when ready+unsealed and traffic strategy satisfied",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: true},
			},
			desiredPods: 3,
			strategy:    openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights,
			trafficStep: 3,
			expectedOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, _ := demotionPreconditionsSatisfied(tt.pods, tt.desiredPods, tt.strategy, tt.trafficStep)
			if gotOK != tt.expectedOK {
				t.Fatalf("ok=%v, expected %v", gotOK, tt.expectedOK)
			}
		})
	}
}

func TestCleanupPreconditionsSatisfied(t *testing.T) {
	tests := []struct {
		name        string
		pods        []podSnapshot
		desiredPods int
		leaderOK    bool
		expectedOK  bool
	}{
		{
			name: "blocks when not fully ready+unsealed",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true, Active: true},
				{Ready: true, Unsealed: false, Active: false},
				{Ready: true, Unsealed: true, Active: false},
			},
			desiredPods: 3,
			leaderOK:    true,
			expectedOK:  false,
		},
		{
			name: "blocks when no leader observed",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true, Active: false},
				{Ready: true, Unsealed: true, Active: false},
				{Ready: true, Unsealed: true, Active: false},
			},
			desiredPods: 3,
			leaderOK:    false,
			expectedOK:  false,
		},
		{
			name: "allows when ready+unsealed and leader observed",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true, Active: true},
				{Ready: true, Unsealed: true, Active: false},
				{Ready: true, Unsealed: true, Active: false},
			},
			desiredPods: 3,
			leaderOK:    true,
			expectedOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, _ := cleanupPreconditionsSatisfied(tt.pods, tt.desiredPods, tt.leaderOK)
			if gotOK != tt.expectedOK {
				t.Fatalf("ok=%v, expected %v", gotOK, tt.expectedOK)
			}
		})
	}
}

func TestPostSwitchHookDecision(t *testing.T) {
	tests := []struct {
		name         string
		autoRollback autoRollbackConfig
		hookJob      *JobResult
		wantHandled  bool
		wantKind     phaseOutcomeKind
	}{
		{
			name:         "nil hook is not handled",
			autoRollback: autoRollbackConfig{Enabled: true, OnTrafficFailure: true},
			hookJob:      nil,
			wantHandled:  false,
			wantKind:     "",
		},
		{
			name:         "running hook triggers short requeue",
			autoRollback: autoRollbackConfig{Enabled: true, OnTrafficFailure: true},
			hookJob:      &JobResult{Name: "hook", Exists: true, Running: true},
			wantHandled:  true,
			wantKind:     phaseOutcomeRequeueAfter,
		},
		{
			name:         "failed hook rolls back when traffic failure rollback enabled",
			autoRollback: autoRollbackConfig{Enabled: true, OnTrafficFailure: true},
			hookJob:      &JobResult{Name: "hook", Exists: true, Failed: true},
			wantHandled:  true,
			wantKind:     phaseOutcomeRollback,
		},
		{
			name:         "failed hook holds when traffic failure rollback disabled",
			autoRollback: autoRollbackConfig{Enabled: true, OnTrafficFailure: false},
			hookJob:      &JobResult{Name: "hook", Exists: true, Failed: true},
			wantHandled:  true,
			wantKind:     phaseOutcomeHold,
		},
		{
			name:         "succeeded hook is not handled",
			autoRollback: autoRollbackConfig{Enabled: true, OnTrafficFailure: true},
			hookJob:      &JobResult{Name: "hook", Exists: true, Succeeded: true},
			wantHandled:  false,
			wantKind:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := postSwitchHookDecision(tt.autoRollback, tt.hookJob, "failed")
			if err != nil {
				t.Fatalf("postSwitchHookDecision: %v", err)
			}
			if got.Handled != tt.wantHandled {
				t.Fatalf("Handled=%v want=%v", got.Handled, tt.wantHandled)
			}
			if got.Outcome.kind != tt.wantKind {
				t.Fatalf("Outcome.kind=%s want=%s", got.Outcome.kind, tt.wantKind)
			}
		})
	}
}

func TestTrafficSwitchServiceSelectorsDecision(t *testing.T) {
	now := time.Unix(1000, 0)

	t.Run("initial switch sets time and requeues", func(t *testing.T) {
		dec, err := trafficSwitchServiceSelectorsDecision(now, nil, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeRequeueAfter {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeRequeueAfter)
		}
		if !dec.SetTrafficSwitchedAt {
			t.Fatalf("expected SetTrafficSwitchedAt")
		}
		if !dec.TrafficSwitchedAt.Equal(now) {
			t.Fatalf("TrafficSwitchedAt=%v want=%v", dec.TrafficSwitchedAt, now)
		}
	})

	t.Run("stabilizing with hook running requeues", func(t *testing.T) {
		switched := now.Add(-10 * time.Second)
		dec, err := trafficSwitchServiceSelectorsDecision(now, &switched, 60, autoRollbackConfig{Enabled: true, OnTrafficFailure: true}, &JobResult{Running: true}, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeRequeueAfter {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeRequeueAfter)
		}
	})

	t.Run("stabilizing with failed hook rolls back when enabled", func(t *testing.T) {
		switched := now.Add(-10 * time.Second)
		dec, err := trafficSwitchServiceSelectorsDecision(now, &switched, 60, autoRollbackConfig{Enabled: true, OnTrafficFailure: true}, &JobResult{Failed: true}, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeRollback {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeRollback)
		}
	})

	t.Run("stabilizing with failed hook holds when disabled", func(t *testing.T) {
		switched := now.Add(-10 * time.Second)
		dec, err := trafficSwitchServiceSelectorsDecision(now, &switched, 60, autoRollbackConfig{Enabled: true, OnTrafficFailure: false}, &JobResult{Failed: true}, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeHold {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeHold)
		}
	})

	t.Run("stabilizing with unhealthy green rolls back when enabled", func(t *testing.T) {
		switched := now.Add(-10 * time.Second)
		dec, err := trafficSwitchServiceSelectorsDecision(now, &switched, 60, autoRollbackConfig{Enabled: true, OnTrafficFailure: true}, &JobResult{Succeeded: true}, false)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeRollback {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeRollback)
		}
	})

	t.Run("after stabilization advances", func(t *testing.T) {
		switched := now.Add(-2 * time.Minute)
		dec, err := trafficSwitchServiceSelectorsDecision(now, &switched, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeAdvance {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeAdvance)
		}
		if dec.Outcome.nextPhase != openbaov1alpha1.PhaseDemotingBlue {
			t.Fatalf("nextPhase=%s want=%s", dec.Outcome.nextPhase, openbaov1alpha1.PhaseDemotingBlue)
		}
	})
}

func TestTrafficSwitchGatewayWeightsDecision(t *testing.T) {
	now := time.Unix(1000, 0)

	t.Run("step 0 moves to step 1 and sets time", func(t *testing.T) {
		dec, err := trafficSwitchGatewayWeightsDecision(now, 0, nil, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if !dec.SetTrafficStep || dec.TrafficStep != 1 {
			t.Fatalf("TrafficStep=%d want=%d", dec.TrafficStep, 1)
		}
		if !dec.SetTrafficSwitchedAt || !dec.TrafficSwitchedAt.Equal(now) {
			t.Fatalf("expected TrafficSwitchedAt to be set to now")
		}
	})

	t.Run("stabilizing requeues", func(t *testing.T) {
		switched := now.Add(-10 * time.Second)
		dec, err := trafficSwitchGatewayWeightsDecision(now, 1, &switched, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeRequeueAfter {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeRequeueAfter)
		}
	})

	t.Run("after stabilization step 1 advances to step 2", func(t *testing.T) {
		switched := now.Add(-2 * time.Minute)
		dec, err := trafficSwitchGatewayWeightsDecision(now, 1, &switched, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if !dec.SetTrafficStep || dec.TrafficStep != 2 {
			t.Fatalf("TrafficStep=%d want=%d", dec.TrafficStep, 2)
		}
		if dec.Outcome.kind != phaseOutcomeRequeueAfter {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeRequeueAfter)
		}
	})

	t.Run("after stabilization step 2 advances to step 3", func(t *testing.T) {
		switched := now.Add(-2 * time.Minute)
		dec, err := trafficSwitchGatewayWeightsDecision(now, 2, &switched, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if !dec.SetTrafficStep || dec.TrafficStep != 3 {
			t.Fatalf("TrafficStep=%d want=%d", dec.TrafficStep, 3)
		}
	})

	t.Run("after stabilization final step advances to DemotingBlue", func(t *testing.T) {
		switched := now.Add(-2 * time.Minute)
		dec, err := trafficSwitchGatewayWeightsDecision(now, 3, &switched, 60, autoRollbackConfig{}, nil, true)
		if err != nil {
			t.Fatalf("decision: %v", err)
		}
		if dec.Outcome.kind != phaseOutcomeAdvance {
			t.Fatalf("Outcome.kind=%s want=%s", dec.Outcome.kind, phaseOutcomeAdvance)
		}
		if dec.Outcome.nextPhase != openbaov1alpha1.PhaseDemotingBlue {
			t.Fatalf("nextPhase=%s want=%s", dec.Outcome.nextPhase, openbaov1alpha1.PhaseDemotingBlue)
		}
	})
}
