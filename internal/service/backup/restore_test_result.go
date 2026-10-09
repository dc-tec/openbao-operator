package backup

import (
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
)

const (
	reasonRestoreTestPreparationFailed      = "PreparationFailed"
	reasonRestoreTestAdministratorAbandoned = "AdministratorAbandoned"
	reasonRestoreTestSnapshotApplied        = "SnapshotApplied"
	reasonRestoreTestApplicationUnconfirmed = "ApplicationUnconfirmed"
	reasonRestoreTestExecutorFailed         = "ExecutorFailed"
	reasonRestoreTestRequestMissing         = "RequestMissing"
	reasonRestoreTestDestinationRejected    = "DestinationRejected"
)

func failedRestoreTestResult(run *api.RestoreTestRun, now time.Time, reason, message string) *api.RestoreTestResult {
	return &api.RestoreTestResult{
		Name: run.Name, Namespace: run.Namespace, Key: run.Key,
		FinishedAt: metav1.NewTime(now), Outcome: api.RestoreTestFailed,
		Reason: reason, Message: message,
	}
}

// Summaries use bounded, operator-defined text. Never persist child messages,
// Job logs, or provider responses, which can contain sensitive data.
func summarizeRestoreTest(run *api.RestoreTestRun, child *api.OpenBaoRestore, now time.Time) *api.RestoreTestResult {
	result := failedRestoreTestResult(run, now, reasonRestoreTestPreparationFailed,
		"Restore preparation failed; check target bootstrap, destination admission, and credentials")
	result.Digest = child.Spec.Source.ExpectedDigest

	switch {
	case child.Status.AdministratorDisposition == api.RestoreAdministratorAbandon:
		result.Reason, result.Message = reasonRestoreTestAdministratorAbandoned, "Administrator accepted responsibility for remaining target resources"
	case child.Status.Phase == api.RestorePhaseCompleted && child.Status.Target != nil && child.Status.Target.AppliedAt != nil:
		result.Outcome = api.RestoreTestPassed
		result.Reason, result.Message = reasonRestoreTestSnapshotApplied, "Fresh target confirmed the expected source identity and completed Kubernetes resource cleanup"
	case child.Status.SubmissionClaim != nil:
		result.Reason, result.Message = reasonRestoreTestApplicationUnconfirmed, "Snapshot submission was claimed, but the expected source identity was not confirmed before cleanup"
	case child.Status.Execution != nil:
		result.Reason, result.Message = reasonRestoreTestExecutorFailed, "Restore execution failed before snapshot submission; check executor logs in your external log store"
	}

	condition := meta.FindStatusCondition(child.Status.Conditions, constants.RestoreConditionType)
	if result.Outcome == api.RestoreTestPassed || child.Status.AdministratorDisposition != "" || condition == nil {
		return result
	}

	// These reasons are part of the restore condition contract. The whitelist
	// preserves useful preparation failures without copying arbitrary status text.
	messages := map[string]string{
		"TargetBootstrapTimedOut": "Fresh target bootstrap exceeded its deadline; check scheduling, storage, unseal access, and readiness",
		"TargetIdentityMismatch":  "Fresh target did not report the expected version and a distinct bootstrap identity",
		"DestinationRejected":     "Destination admission rejected the fresh target; check namespace approval and configured resources",
		"TargetCollision":         "A cluster or data PVC already used the requested fresh target name",
		"TargetStorageChanged":    "Fresh target data PVC was replaced or lacked original ownership proof",
		"TargetUnavailable":       "The reserved fresh target was missing, replaced, or deleting",
	}
	if message, known := messages[condition.Reason]; known {
		result.Reason, result.Message = condition.Reason, message
	}
	return result
}
