package bluegreen

import (
	"testing"
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
			expectedOK:  false,
		},
		{
			name: "allows when fully ready+unsealed",
			pods: []podSnapshot{
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: true},
				{Ready: true, Unsealed: true},
			},
			desiredPods: 3,
			expectedOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, _ := demotionPreconditionsSatisfied(tt.pods, tt.desiredPods)
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
