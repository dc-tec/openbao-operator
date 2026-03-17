package bluegreen

import (
	"strings"
	"testing"

	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func FuzzBlueGreenStepperHelpers(f *testing.F) {
	f.Add(uint8(0), uint8(0), int32(0), int32(2), "abort")
	f.Add(uint8(1), uint8(1), int32(1), int32(1), "rollback")

	f.Fuzz(func(t *testing.T, podSeed, jobSeed uint8, currentFailures, maxFailures int32, failureReason string) {
		pods := fuzzPodSnapshots(podSeed)
		_ = countReadyUnsealedPods(pods)
		_ = leaderObserved(pods)
		_, _ = demotionPreconditionsSatisfied(pods, int(currentFailures))
		_, _ = cleanupPreconditionsSatisfied(pods, int(maxFailures), podSeed%2 == 0)

		autoRollback := autoRollbackConfig{
			Enabled:             podSeed%2 == 0,
			OnJobFailure:        jobSeed%2 == 0,
			OnValidationFailure: jobSeed%3 == 0,
		}
		result := fuzzJobResult(jobSeed)

		decision, err := executorJobDecision(autoRollback, clampStepFailures(currentFailures), clampStepFailures(maxFailures), result, sanitizeStepReason(failureReason, "abort"))
		if result == nil {
			if err == nil {
				t.Fatalf("expected nil job result to fail")
			}
		} else if !result.Succeeded && !result.Running && !result.Failed {
			if err == nil {
				t.Fatalf("expected invalid job result to fail")
			}
		} else if err != nil {
			t.Fatalf("executorJobDecision() error = %v", err)
		} else if decision.JobFailed && decision.LastJobFailure == "" {
			t.Fatalf("failed job decisions must record the last failing job")
		}

		hookDecision, err := validationHookDecision(autoRollback.OnValidationFailure, result, sanitizeStepReason(failureReason, "hook failed"))
		if result != nil && !result.Succeeded && !result.Running && !result.Failed {
			if err == nil {
				t.Fatalf("expected invalid hook result to fail")
			}
		} else if err != nil {
			t.Fatalf("validationHookDecision() error = %v", err)
		} else if hookDecision.Handled {
			if err := hookDecision.Outcome.validate(); err != nil {
				t.Fatalf("hook outcome validation failed: %v", err)
			}
		}

		preDecision, err := prePromotionHookDecision(autoRollback, result, sanitizeStepReason(failureReason, "pre-promotion"))
		if result != nil && !result.Succeeded && !result.Running && !result.Failed {
			if err == nil {
				t.Fatalf("expected invalid pre-promotion hook result to fail")
			}
		} else if err != nil {
			t.Fatalf("prePromotionHookDecision() error = %v", err)
		} else if preDecision.Handled {
			if err := preDecision.Outcome.validate(); err != nil {
				t.Fatalf("pre-promotion outcome validation failed: %v", err)
			}
		}

		_ = executorRunID(autoRollback, clampStepFailures(currentFailures))
	})
}

func fuzzPodSnapshots(seed uint8) []podSnapshot {
	count := int(seed%4) + 1
	pods := make([]podSnapshot, 0, count)
	for i := 0; i < count; i++ {
		mask := uint8(i + 1)
		pods = append(pods, podSnapshot{
			Ready:    seed&mask != 0,
			Unsealed: seed&(mask<<1) != 0,
			Active:   i == 0 && seed%2 == 0,
			Deleting: seed&(mask<<2) != 0,
		})
	}
	return pods
}

func fuzzJobResult(seed uint8) *upgrade.JobResult {
	switch seed % 5 {
	case 0:
		return nil
	case 1:
		return &upgrade.JobResult{Name: "job", Running: true}
	case 2:
		return &upgrade.JobResult{Name: "job", Failed: true}
	case 3:
		return &upgrade.JobResult{Name: "job", Succeeded: true}
	default:
		return &upgrade.JobResult{Name: "job"}
	}
}

func clampStepFailures(value int32) int32 {
	if value < 0 {
		return 0
	}
	if value == 0 {
		return 1
	}
	return value % 6
}

func sanitizeStepReason(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 120 {
		return trimmed[:120]
	}
	return trimmed
}
