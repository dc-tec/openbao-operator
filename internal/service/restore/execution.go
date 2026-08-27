package restore

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const restoreExecutionIDAnnotation = "openbao.org/restore-execution-id"

func restoreExecutionOperationID(restore *openbaov1alpha1.OpenBaoRestore) string {
	if restore == nil {
		return ""
	}
	if restore.UID != "" {
		return string(restore.UID)
	}
	return fmt.Sprintf("%s/%s", restore.Namespace, restore.Name)
}

func newRestoreExecutionStatus(restore *openbaov1alpha1.OpenBaoRestore) *openbaov1alpha1.RestoreExecutionStatus {
	now := metav1.Now()
	return &openbaov1alpha1.RestoreExecutionStatus{
		OperationID: restoreExecutionOperationID(restore),
		Stage:       openbaov1alpha1.RestoreExecutionStagePrepared,
		JobName:     restoreJobName(restore),
		PreparedAt:  &now,
	}
}

func restoreExecutionCommitted(execution *openbaov1alpha1.RestoreExecutionStatus) bool {
	if execution == nil {
		return false
	}

	switch execution.Stage {
	case openbaov1alpha1.RestoreExecutionStageCommitted,
		openbaov1alpha1.RestoreExecutionStageCreated,
		openbaov1alpha1.RestoreExecutionStageTerminalObserved,
		openbaov1alpha1.RestoreExecutionStageFollowThroughComplete,
		openbaov1alpha1.RestoreExecutionStageUnknown:
		return true
	default:
		return false
	}
}

func validateRestoreExecutionIdentity(restore *openbaov1alpha1.OpenBaoRestore) error {
	if restore.Status.Execution == nil {
		return fmt.Errorf("restore execution status is missing")
	}
	execution := restore.Status.Execution
	if execution.OperationID != restoreExecutionOperationID(restore) {
		return fmt.Errorf("restore execution operationID %q does not match expected %q", execution.OperationID, restoreExecutionOperationID(restore))
	}
	if execution.JobName != restoreJobName(restore) {
		return fmt.Errorf("restore execution jobName %q does not match expected %q", execution.JobName, restoreJobName(restore))
	}
	return nil
}

func validateRestoreExecutionJob(execution *openbaov1alpha1.RestoreExecutionStatus, job *batchv1.Job) error {
	if execution == nil || job == nil {
		return fmt.Errorf("restore execution and Job are required")
	}
	if job.Name != execution.JobName {
		return fmt.Errorf("restore Job name %q does not match receipt %q", job.Name, execution.JobName)
	}
	if execution.JobUID != "" && job.UID != execution.JobUID {
		return fmt.Errorf("restore Job UID %q does not match receipt %q", job.UID, execution.JobUID)
	}
	if operationID := job.Annotations[restoreExecutionIDAnnotation]; operationID != "" && operationID != execution.OperationID {
		return fmt.Errorf("restore Job operation ID %q does not match receipt %q", operationID, execution.OperationID)
	}
	return nil
}

func (m *Manager) markRestoreExecutionCommitted(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore) error {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Execution.Stage = openbaov1alpha1.RestoreExecutionStageCommitted
	restore.Status.Execution.CommittedAt = &now
	restore.Status.Message = fmt.Sprintf("Committed restore execution %s; creating Job %s exactly once.", restore.Status.Execution.OperationID, restore.Status.Execution.JobName)
	return m.patchStatus(ctx, restore, original)
}

func (m *Manager) markRestoreExecutionCreated(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, job *batchv1.Job) error {
	if err := validateRestoreExecutionJob(restore.Status.Execution, job); err != nil {
		return err
	}

	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Execution.Stage = openbaov1alpha1.RestoreExecutionStageCreated
	restore.Status.Execution.JobUID = job.UID
	restore.Status.Execution.CreatedAt = &now
	restore.Status.Message = restoreJobRunningStatusMessage(job.Name)
	return m.patchStatus(ctx, restore, original)
}

func (m *Manager) markRestoreExecutionTerminal(
	ctx context.Context,
	restore *openbaov1alpha1.OpenBaoRestore,
	result openbaov1alpha1.RestoreExecutionResult,
) error {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Execution.Stage = openbaov1alpha1.RestoreExecutionStageTerminalObserved
	restore.Status.Execution.TerminalResult = result
	restore.Status.Execution.TerminalObservedAt = &now
	restore.Status.Message = fmt.Sprintf("Restore Job %s terminal result %s recorded.", restore.Status.Execution.JobName, result)
	return m.patchStatus(ctx, restore, original)
}

func (m *Manager) markRestoreFollowThroughComplete(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore) error {
	original := restore.DeepCopy()
	now := metav1.Now()
	restore.Status.Execution.Stage = openbaov1alpha1.RestoreExecutionStageFollowThroughComplete
	restore.Status.Execution.FollowThroughCompletedAt = &now
	restore.Status.Message = "Post-restore voter and read-replica recovery completed."
	return m.patchStatus(ctx, restore, original)
}

func (m *Manager) markRestoreExecutionUnknown(ctx context.Context, restore *openbaov1alpha1.OpenBaoRestore, message string) error {
	original := restore.DeepCopy()
	now := metav1.Now()
	if restore.Status.Execution == nil {
		restore.Status.Execution = newRestoreExecutionStatus(restore)
	}
	restore.Status.Execution.Stage = openbaov1alpha1.RestoreExecutionStageUnknown
	restore.Status.Phase = openbaov1alpha1.RestorePhaseUnknown
	restore.Status.Message = message
	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               string(RestoreConditionType),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: restore.Generation,
		Reason:             ReasonRestoreExecutionUnknown,
		Message:            message,
		LastTransitionTime: now,
	})
	return m.patchStatus(ctx, restore, original)
}
