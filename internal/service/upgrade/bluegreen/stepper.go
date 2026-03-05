package bluegreen

import (
	"fmt"

	"github.com/dc-tec/openbao-operator/internal/constants"
)

type podSnapshot struct {
	Ready    bool
	Unsealed bool
	Active   bool
	Deleting bool
}

func countReadyUnsealedPods(pods []podSnapshot) int {
	ready := 0
	for i := range pods {
		pod := &pods[i]
		if pod.Deleting {
			continue
		}
		if pod.Ready && pod.Unsealed {
			ready++
		}
	}
	return ready
}

func leaderObserved(pods []podSnapshot) bool {
	for i := range pods {
		pod := &pods[i]
		if pod.Deleting {
			continue
		}
		if pod.Active {
			return true
		}
	}
	return false
}

func demotionPreconditionsSatisfied(
	greenPods []podSnapshot,
	desiredGreenPods int,
) (ok bool, message string) {
	if desiredGreenPods < 1 {
		desiredGreenPods = 1
	}

	readyUnsealed := countReadyUnsealedPods(greenPods)
	if readyUnsealed < desiredGreenPods {
		return false, fmt.Sprintf("blocking demotion: ready+unsealed Green pods %d < desired %d", readyUnsealed, desiredGreenPods)
	}

	return true, ""
}

func cleanupPreconditionsSatisfied(greenPods []podSnapshot, desiredGreenPods int, leaderOK bool) (ok bool, message string) {
	if desiredGreenPods < 1 {
		desiredGreenPods = 1
	}

	readyUnsealed := countReadyUnsealedPods(greenPods)
	if readyUnsealed < desiredGreenPods {
		return false, fmt.Sprintf("blocking cleanup: ready+unsealed Green pods %d < desired %d", readyUnsealed, desiredGreenPods)
	}

	if !leaderOK {
		return false, "blocking cleanup: green leader not yet observed"
	}

	return true, ""
}

func executorRunID(autoRollback autoRollbackConfig, currentFailureCount int32) string {
	shouldRetryOrRollback := autoRollback.Enabled && autoRollback.OnJobFailure
	if !shouldRetryOrRollback || currentFailureCount <= 0 {
		return ""
	}
	return fmt.Sprintf("retry-%d", currentFailureCount)
}

type executorDecision struct {
	Completed        bool
	Outcome          phaseOutcome
	NextFailureCount int32
	JobFailed        bool
	LastJobFailure   string
}

func executorJobDecision(
	autoRollback autoRollbackConfig,
	currentFailureCount int32,
	maxFailures int32,
	result *JobResult,
	abortMessage string,
) (executorDecision, error) {
	if result == nil {
		return executorDecision{}, fmt.Errorf("job result is required")
	}
	if maxFailures <= 0 {
		maxFailures = 5
	}

	if result.Succeeded {
		return executorDecision{
			Completed:        true,
			NextFailureCount: 0,
			JobFailed:        false,
			LastJobFailure:   "",
		}, nil
	}

	if result.Running {
		return executorDecision{
			Outcome:          requeueAfterOutcome(constants.RequeueShort),
			NextFailureCount: currentFailureCount,
			JobFailed:        false,
			LastJobFailure:   "",
		}, nil
	}

	if !result.Failed {
		return executorDecision{}, fmt.Errorf("invalid job result: neither running, failed, nor succeeded")
	}

	shouldRetryOrRollback := autoRollback.Enabled && autoRollback.OnJobFailure

	decision := executorDecision{
		Completed:      false,
		JobFailed:      true,
		LastJobFailure: result.Name,
	}

	if shouldRetryOrRollback {
		nextCount := currentFailureCount + 1
		decision.NextFailureCount = nextCount

		if nextCount >= maxFailures {
			decision.Outcome = rollback(abortMessage)
			return decision, nil
		}

		decision.Outcome = requeueAfterOutcome(constants.RequeueShort)
		return decision, nil
	}

	if currentFailureCount <= 0 {
		decision.NextFailureCount = 1
	} else {
		decision.NextFailureCount = currentFailureCount
	}
	decision.Outcome = hold()
	return decision, nil
}

type hookDecision struct {
	Handled bool
	Outcome phaseOutcome
}

func validationHookDecision(rollbackOnFailure bool, hookJob *JobResult, failureReason string) (hookDecision, error) {
	if hookJob == nil {
		return hookDecision{Handled: false}, nil
	}

	if hookJob.Running {
		return hookDecision{Handled: true, Outcome: requeueAfterOutcome(constants.RequeueShort)}, nil
	}
	if hookJob.Failed {
		if rollbackOnFailure {
			return hookDecision{Handled: true, Outcome: rollback(failureReason)}, nil
		}
		return hookDecision{Handled: true, Outcome: hold()}, nil
	}
	if hookJob.Succeeded {
		return hookDecision{Handled: false}, nil
	}

	return hookDecision{}, fmt.Errorf("invalid hook job result: neither running, failed, nor succeeded")
}

func prePromotionHookDecision(autoRollback autoRollbackConfig, hookJob *JobResult, failureReason string) (hookDecision, error) {
	return validationHookDecision(autoRollback.Enabled && autoRollback.OnValidationFailure, hookJob, failureReason)
}
