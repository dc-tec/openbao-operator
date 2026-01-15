package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

type executorJobStep struct {
	Completed bool
	Outcome   phaseOutcome
}

func (m *Manager) runExecutorJobStep(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, action ExecutorAction, abortMessage string) (executorJobStep, error) {
	if cluster.Status.BlueGreen == nil {
		return executorJobStep{}, fmt.Errorf("blue/green status is nil")
	}

	autoRollback := autoRollbackSettings(cluster)
	runID := executorRunID(autoRollback, cluster.Status.BlueGreen.JobFailureCount)

	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		action,
		runID,
		cluster.Status.BlueGreen.BlueRevision,
		cluster.Status.BlueGreen.GreenRevision,
		m.clientConfig,
	)
	if err != nil {
		return executorJobStep{}, err
	}

	decision, err := executorJobDecision(autoRollback, cluster.Status.BlueGreen.JobFailureCount, m.getMaxJobFailures(cluster), result, abortMessage)
	if err != nil {
		return executorJobStep{}, err
	}

	if decision.Completed {
		cluster.Status.BlueGreen.JobFailureCount = 0
		cluster.Status.BlueGreen.LastJobFailure = ""
		return executorJobStep{Completed: true}, nil
	}

	if decision.JobFailed {
		cluster.Status.BlueGreen.LastJobFailure = decision.LastJobFailure

		shouldRetryOrRollback := autoRollback.Enabled && autoRollback.OnJobFailure
		if shouldRetryOrRollback {
			logger.Info("Job failure recorded",
				"job", decision.LastJobFailure,
				"failureCount", decision.NextFailureCount,
				"maxFailures", m.getMaxJobFailures(cluster),
				"action", action)
		} else {
			logger.Info("Upgrade executor Job failed; auto rollback/retry disabled, pausing",
				"job", decision.LastJobFailure,
				"action", action,
				"autoRollbackEnabled", autoRollback.Enabled,
				"onJobFailure", autoRollback.OnJobFailure)
		}
		cluster.Status.BlueGreen.JobFailureCount = decision.NextFailureCount
	}

	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", action)
	}

	return executorJobStep{Outcome: decision.Outcome}, nil
}
