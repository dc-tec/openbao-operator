package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func FuzzExecutorPolicyHelpers(f *testing.F) {
	f.Add("green", "blue", "primary", "fallback", uint8(0))
	f.Add("rev-a", "rev-a", "", "", uint8(1))

	f.Fuzz(func(t *testing.T, primaryRevision, fallbackRevision, primaryLabel, fallbackLabel string, outcomeSeed uint8) {
		policy := newLeaderSearchPolicy(primaryRevision, fallbackRevision, primaryLabel, fallbackLabel)
		_ = maxLeaderSearchAttempts(policy)

		cfg := &ExecutorConfig{
			ClusterNamespace: "default",
			ClusterName:      "cluster",
			ClusterReplicas:  3,
			Action:           ExecutorActionRollingStepDownLeader,
			JWTAuthRole:      "role",
			JWTToken:         "token",
			TLSCACert:        []byte("ca"),
			SyncThreshold:    1,
			Timeout:          1,
		}

		outcome := resolveLeaderWithPolicyUsing(context.Background(), cfg, policy, func(_ context.Context, _ *ExecutorConfig, revision string) (string, error) {
			switch outcomeSeed % 4 {
			case 0:
				return "https://" + sanitizeUpgradeToken(revision, "leader"), nil
			case 1:
				return "", context.Canceled
			case 2:
				return "", context.DeadlineExceeded
			default:
				return "", errors.New("not found")
			}
		})

		if outcome.AttemptsUsed < 1 || outcome.AttemptsUsed > 2 {
			t.Fatalf("unexpected attempts used %d", outcome.AttemptsUsed)
		}
		if outcome.Value == "" && outcome.ReasonCode == "" {
			t.Fatalf("expected either leader value or reason code")
		}
		if outcome.PrimaryError != nil {
			_ = reasonCodeFromError(newExecutorReasonedError(reasonCodeFromContextError(outcome.PrimaryError), "wrapped", outcome.PrimaryError))
		}
		_ = decisionPathFromReasonCode(outcome.ReasonCode)
	})
}

func sanitizeUpgradeToken(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.ReplaceAll(trimmed, "\x00", "")
	trimmed = strings.ReplaceAll(trimmed, " ", "-")
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 32 {
		return trimmed[:32]
	}
	return trimmed
}
