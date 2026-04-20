package restore

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

func restoreOperationLock(restore *openbaov1alpha1.OpenBaoRestore) opslifecycle.OperationLock {
	return opslifecycle.OperationLock{
		Holder:    fmt.Sprintf("%s/%s", constants.ControllerNameOpenBaoRestore, restore.Name),
		Operation: openbaov1alpha1.ClusterOperationRestore,
	}
}

func restoreLockMessage(restore *openbaov1alpha1.OpenBaoRestore) string {
	return fmt.Sprintf("restore %s/%s", restore.Namespace, restore.Name)
}

func restoreWaitingForOperationLockStatusMessage(err error) string {
	if held, ok := opslifecycle.AsHeldError(err); ok {
		return fmt.Sprintf(
			"Waiting for cluster operation lock held by operation=%s holder=%s. Restore will retry automatically; use overrideOperationLock with force=true only for disaster recovery.",
			held.Operation,
			held.Holder,
		)
	}

	return "Waiting for cluster operation lock. Restore will retry automatically."
}

func restoreJobRunningStatusMessage(jobName string) string {
	return fmt.Sprintf("Restore Job %s is running; waiting for completion.", jobName)
}

func restoreJobFailedStatusMessage(job *batchv1.Job, failureHint string) string {
	message := ""
	if job == nil {
		message = "Restore Job failed. Check the restore Job logs and create a new OpenBaoRestore to retry."
	} else {
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue && cond.Message != "" {
				message = fmt.Sprintf(
					"Restore Job %s failed: %s. Check kubectl logs job/%s -n %s and create a new OpenBaoRestore to retry.",
					job.Name,
					cond.Message,
					job.Name,
					job.Namespace,
				)
				break
			}
		}
	}

	if message == "" && job != nil {
		message = fmt.Sprintf(
			"Restore Job %s failed. Check kubectl logs job/%s -n %s and create a new OpenBaoRestore to retry.",
			job.Name,
			job.Name,
			job.Namespace,
		)
	}

	if failureHint == "" {
		return message
	}

	return message + " " + failureHint
}

// restoreJobName returns the name for the restore job.
func restoreJobName(restore *openbaov1alpha1.OpenBaoRestore) string {
	return fmt.Sprintf("%s%s", RestoreJobNamePrefix, restore.Name)
}
