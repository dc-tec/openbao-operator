package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (m *Manager) runExecutorJob(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, action ExecutorAction, abortMessage string) (bool, error) {
	if cluster.Status.BlueGreen == nil {
		return false, fmt.Errorf("blue/green status is nil")
	}

	result, err := EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		action,
		"",
		cluster.Status.BlueGreen.BlueRevision,
		cluster.Status.BlueGreen.GreenRevision,
	)
	if err != nil {
		return false, err
	}
	if result.Failed {
		if shouldAbort := m.handleJobFailure(logger, cluster, result.Name); shouldAbort {
			return m.triggerRollbackOrAbort(ctx, logger, cluster, abortMessage)
		}
		return true, nil
	}
	if result.Running {
		logger.Info("Upgrade executor Job is in progress", "job", result.Name, "action", action)
		return true, nil
	}

	cluster.Status.BlueGreen.JobFailureCount = 0
	cluster.Status.BlueGreen.LastJobFailure = ""

	return false, nil
}
