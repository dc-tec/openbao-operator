package raftops

import (
	"context"
	"errors"
	"testing"
)

func TestResolveLeaderWithPolicyUsing(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{}

	t.Run("primary success", func(t *testing.T) {
		t.Parallel()

		policy := NewLeaderSearchPolicy("blue", "green", "Blue", "Green")
		outcome := ResolveLeaderWithPolicyUsing(context.Background(), cfg, policy, func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
			if revision == "blue" {
				return "https://blue-leader", nil
			}
			return "", errors.New("unexpected revision")
		})

		if outcome.Value != "https://blue-leader" {
			t.Fatalf("Value = %q, want primary leader URL", outcome.Value)
		}
		if outcome.DecisionPath != DecisionPathPrimarySuccess {
			t.Fatalf("DecisionPath = %q, want %q", outcome.DecisionPath, DecisionPathPrimarySuccess)
		}
		if outcome.ReasonCode != ReasonPrimaryLeaderFound {
			t.Fatalf("ReasonCode = %q, want %q", outcome.ReasonCode, ReasonPrimaryLeaderFound)
		}
		if outcome.AttemptsUsed != 1 {
			t.Fatalf("AttemptsUsed = %d, want 1", outcome.AttemptsUsed)
		}
	})

	t.Run("fallback success", func(t *testing.T) {
		t.Parallel()

		policy := NewLeaderSearchPolicy("blue", "green", "Blue", "Green")
		outcome := ResolveLeaderWithPolicyUsing(context.Background(), cfg, policy, func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
			if revision == "blue" {
				return "", errors.New("blue missing leader")
			}
			return "https://green-leader", nil
		})

		if outcome.Value != "https://green-leader" {
			t.Fatalf("Value = %q, want fallback leader URL", outcome.Value)
		}
		if outcome.DecisionPath != DecisionPathPrimaryFailedFallbackSuccess {
			t.Fatalf("DecisionPath = %q, want %q", outcome.DecisionPath, DecisionPathPrimaryFailedFallbackSuccess)
		}
		if outcome.ReasonCode != ReasonFallbackLeaderFound {
			t.Fatalf("ReasonCode = %q, want %q", outcome.ReasonCode, ReasonFallbackLeaderFound)
		}
		if outcome.AttemptsUsed != 2 {
			t.Fatalf("AttemptsUsed = %d, want 2", outcome.AttemptsUsed)
		}
	})

	t.Run("no fallback keeps primary error shape", func(t *testing.T) {
		t.Parallel()

		policy := NewLeaderSearchPolicy("blue", "blue", "Blue", "Blue")
		outcome := ResolveLeaderWithPolicyUsing(context.Background(), cfg, policy, func(_ context.Context, _ *ExecutorConfig, _ string) (string, error) {
			return "", errors.New("not found")
		})

		if outcome.Value != "" {
			t.Fatalf("Value = %q, want empty", outcome.Value)
		}
		if outcome.DecisionPath != DecisionPathPrimaryFailedNoFallback {
			t.Fatalf("DecisionPath = %q, want %q", outcome.DecisionPath, DecisionPathPrimaryFailedNoFallback)
		}
		if outcome.ReasonCode != ReasonPrimaryLeaderNotFound {
			t.Fatalf("ReasonCode = %q, want %q", outcome.ReasonCode, ReasonPrimaryLeaderNotFound)
		}
		if outcome.AttemptsUsed != 1 {
			t.Fatalf("AttemptsUsed = %d, want 1", outcome.AttemptsUsed)
		}
	})

	t.Run("context cancellation is preserved", func(t *testing.T) {
		t.Parallel()

		policy := NewLeaderSearchPolicy("blue", "", "Blue", "")
		outcome := ResolveLeaderWithPolicyUsing(context.Background(), cfg, policy, func(_ context.Context, _ *ExecutorConfig, _ string) (string, error) {
			return "", context.Canceled
		})

		if outcome.DecisionPath != DecisionPathContextCanceled {
			t.Fatalf("DecisionPath = %q, want %q", outcome.DecisionPath, DecisionPathContextCanceled)
		}
		if outcome.ReasonCode != ReasonContextCanceled {
			t.Fatalf("ReasonCode = %q, want %q", outcome.ReasonCode, ReasonContextCanceled)
		}
	})
}

func TestErrorClassificationHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  BenignErrorClassification
		want BenignErrorClassification
	}{
		{
			name: "join already joined is benign",
			got:  ClassifyJoinError(errors.New("node already joined to cluster")),
			want: BenignErrorClassificationBenign,
		},
		{
			name: "join permission denied is fatal",
			got:  ClassifyJoinError(errors.New("permission denied")),
			want: BenignErrorClassificationFatal,
		},
		{
			name: "demote already non voter is benign",
			got:  ClassifyDemoteError(errors.New("peer is already a non-voter")),
			want: BenignErrorClassificationBenign,
		},
		{
			name: "demote forbidden is fatal",
			got:  ClassifyDemoteError(errors.New("forbidden")),
			want: BenignErrorClassificationFatal,
		},
		{
			name: "demote generic error is retryable",
			got:  ClassifyDemoteError(errors.New("temporary demote failure")),
			want: BenignErrorClassificationRetryable,
		},
		{
			name: "stepdown generic error is retryable",
			got:  ClassifyStepDownError(errors.New("connection reset")),
			want: BenignErrorClassificationRetryable,
		},
		{
			name: "stepdown unauthorized is fatal",
			got:  ClassifyStepDownError(errors.New("unauthorized")),
			want: BenignErrorClassificationFatal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Fatalf("classification = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
