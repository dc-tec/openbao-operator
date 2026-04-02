package snapshot

import "github.com/go-logr/logr"

// ExistingJobHandlers lets callers keep strategy-specific retry or event logic
// while sharing the status dispatch for an existing pre-upgrade snapshot Job.
type ExistingJobHandlers struct {
	OnFound     func(string)
	OnRunning   func(string)
	OnFailed    func(string) (bool, error)
	OnSucceeded func(string) (bool, error)
}

// ReconcileExistingJob dispatches existing snapshot Job state to the supplied
// handlers. The returned boolean indicates whether the pre-upgrade snapshot is
// complete for the caller's flow.
func ReconcileExistingJob(logger logr.Logger, jobName string, state JobState, handlers ExistingJobHandlers) (bool, error) {
	if handlers.OnFound != nil {
		handlers.OnFound(jobName)
	} else {
		logger.Info("Found existing pre-upgrade snapshot job, checking status", "job", jobName)
	}

	switch state {
	case JobStateFailed:
		if handlers.OnFailed == nil {
			return false, nil
		}
		return handlers.OnFailed(jobName)
	case JobStateSucceeded:
		if handlers.OnSucceeded == nil {
			return true, nil
		}
		return handlers.OnSucceeded(jobName)
	case JobStateRunning:
		if handlers.OnRunning != nil {
			handlers.OnRunning(jobName)
		} else {
			logger.Info("Pre-upgrade snapshot job is still running", "job", jobName)
		}
		return false, nil
	default:
		return false, nil
	}
}
