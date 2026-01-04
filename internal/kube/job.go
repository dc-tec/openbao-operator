package kube

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// JobSucceeded reports whether a Job has completed successfully.
func JobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}

	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}

	return job.Status.Succeeded > 0
}

// JobFailed reports whether a Job has completed unsuccessfully.
func JobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}

	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}

	// Fallback: if the Job has failures and no active pods, treat as failed.
	// This matches the terminal failed state for Jobs where conditions might not
	// be set or are not yet observed.
	return job.Status.Failed > 0 && job.Status.Active == 0 && job.Status.Succeeded == 0
}
