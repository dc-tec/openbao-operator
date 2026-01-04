//go:build e2e
// +build e2e

package e2e

import (
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func jobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	if job.Status.Succeeded > 0 {
		return true
	}
	for _, cond := range job.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		if string(cond.Type) == "Complete" || string(cond.Type) == "SuccessCriteriaMet" {
			return true
		}
	}
	return false
}

func jobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}
	if job.Status.Failed > 0 {
		return true
	}
	for _, cond := range job.Status.Conditions {
		if cond.Status != corev1.ConditionTrue {
			continue
		}
		if string(cond.Type) == "Failed" || string(cond.Type) == "FailureTarget" {
			return true
		}
	}
	return false
}
