package snapshot

import (
	batchv1 "k8s.io/api/batch/v1"

	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// JobState classifies the state of a pre-upgrade snapshot Job.
type JobState string

const (
	JobStateNone      JobState = ""
	JobStateRunning   JobState = "running"
	JobStateFailed    JobState = "failed"
	JobStateSucceeded JobState = "succeeded"
)

// JobStateFromBatchJob classifies a Kubernetes Job.
func JobStateFromBatchJob(job *batchv1.Job) JobState {
	if job == nil {
		return JobStateNone
	}
	if kube.JobSucceeded(job) {
		return JobStateSucceeded
	}
	if kube.JobFailed(job) {
		return JobStateFailed
	}
	return JobStateRunning
}

// JobStateFromResult classifies a generic upgrade Job result.
func JobStateFromResult(result *upgrade.JobResult) JobState {
	if result == nil || !result.Exists {
		return JobStateNone
	}
	if result.Succeeded {
		return JobStateSucceeded
	}
	if result.Failed {
		return JobStateFailed
	}
	if result.Running {
		return JobStateRunning
	}
	return JobStateNone
}
