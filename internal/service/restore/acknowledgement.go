package restore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
	"github.com/kubebao/openbao-operator/internal/service/opslifecycle"
)

func (m *Manager) holdTarget(
	ctx context.Context, request *openbaov1alpha1.OpenBaoRestore,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	live := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.reader.Get(ctx, client.ObjectKeyFromObject(cluster), live); err != nil {
		return err
	}

	marker := restoreExecutionOperationID(request)
	if live.UID != cluster.UID || live.DeletionTimestamp != nil ||
		!restoreOperationLock(request).IsHeldBy(live.Status.OperationLock) {
		return fmt.Errorf("restore requires the original locked target")
	}

	if value := live.Annotations[constants.AnnotationRestoreHold]; value != "" {
		if value != marker {
			return fmt.Errorf("target belongs to another held restore")
		}
		return nil
	}

	before := live.DeepCopy()
	if live.Annotations == nil {
		live.Annotations = make(map[string]string)
	}

	live.Annotations[constants.AnnotationRestoreHold] = marker
	return m.client.Patch(ctx, live, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{}))
}

func (m *Manager) reconcileAcknowledgement(
	ctx context.Context, logger logr.Logger,
	request *openbaov1alpha1.OpenBaoRestore,
) (ctrl.Result, error) {
	cleanupFailed := request.Status.Target != nil && request.Status.Target.Cleanup == openbaov1alpha1.RestoreTargetCleanupFailed
	if request.Status.Phase != openbaov1alpha1.RestorePhaseUnknown && !cleanupFailed {
		return ctrl.Result{}, nil
	}

	value, valid := strings.CutPrefix(request.Annotations[constants.AnnotationRestoreAcknowledge], restoreExecutionOperationID(request)+"/")
	action := openbaov1alpha1.RestoreAdministratorDisposition(value)
	// Once persisted, Resume continues across annotation removal and controller
	// restarts. A matching Abandon can still stop recovery before hold release.
	if request.Status.Restart != nil && request.Status.AdministratorDisposition == "" &&
		(!valid || action != openbaov1alpha1.RestoreAdministratorAbandon) {
		action, valid = openbaov1alpha1.RestoreAdministratorResume, true
	}
	if !valid || (action != openbaov1alpha1.RestoreAdministratorResume && action != openbaov1alpha1.RestoreAdministratorAbandon) {
		return ctrl.Result{}, nil
	}

	if cleanupFailed && action == openbaov1alpha1.RestoreAdministratorResume {
		return m.blockAcknowledgement(ctx, request, "Target cleanup failed; use Abandon after administrator cleanup")
	}
	if request.Spec.TargetLifecycle == openbaov1alpha1.RestoreTargetLifecycleDisposable && action == openbaov1alpha1.RestoreAdministratorResume {
		return m.blockAcknowledgement(ctx, request, "Disposable targets cannot resume; use Abandon after administrator cleanup")
	}

	if request.Status.AdministratorDisposition == "" {
		blocked, err := m.releaseAcknowledgedTarget(ctx, request, action)
		if err != nil {
			if action == openbaov1alpha1.RestoreAdministratorResume {
				var issue *recoveryIssue
				if errors.As(err, &issue) {
					result, patchErr := m.reportRecovery(ctx, request, issue.reason, issue.message)
					return result, errors.Join(issue.cause, patchErr)
				}
				result, patchErr := m.blockAcknowledgement(ctx, request,
					"Resume is waiting for recovery prerequisites; inspect the controller error and repair the target, or use Abandon for administrator recovery")
				return result, errors.Join(err, patchErr)
			}
			return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, err
		}
		if blocked != "" {
			return m.blockAcknowledgement(ctx, request, blocked)
		}

		// This records administrator responsibility, never proof of snapshot application.
		before := request.DeepCopy()
		request.Status.AdministratorDisposition = action
		if action == openbaov1alpha1.RestoreAdministratorResume {
			request.Status.Message = "Administrator accepted recovery; voter and read-replica restarts completed and management resumed. Snapshot application remains unconfirmed for existing targets"
		}
		if action == openbaov1alpha1.RestoreAdministratorAbandon {
			request.Status.Message = "Administrator abandoned operator recovery; the original target is left paused if present"
		}
		if cleanupFailed {
			request.Status.Phase = openbaov1alpha1.RestorePhaseFailed
		}
		now := metav1.Now()
		request.Status.CompletionTime = &now
		reason := ReasonRecoveryResumed
		if action == openbaov1alpha1.RestoreAdministratorAbandon {
			reason = ReasonRecoveryAbandoned
		}
		meta.SetStatusCondition(&request.Status.Conditions, metav1.Condition{Type: constants.RestoreRecoveryReleasedConditionType, Status: metav1.ConditionTrue,
			Reason: reason, Message: "Administrator accepted responsibility for remaining processes and resources; this is not application or deletion proof.",
			ObservedGeneration: request.Generation})
		if err := m.patchStatus(ctx, request, before); err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("Administrator released restore management hold", "restore_name", request.Name, "disposition", action)
	}

	if retainedTargetAccepted(request) {
		return ctrl.Result{}, m.completeRestore(ctx, logger, request, "Fresh target application confirmed; administrator accepted the retained target")
	}

	return ctrl.Result{}, nil
}

func (m *Manager) blockAcknowledgement(ctx context.Context, request *openbaov1alpha1.OpenBaoRestore, message string) (ctrl.Result, error) {
	return m.reportRecovery(ctx, request, ReasonRecoveryBlocked, message)
}

// Abandon can release a request after its target was replaced. It never changes
// the replacement. The administrator assumes responsibility for remaining resources.
func (m *Manager) releaseAcknowledgedTarget(ctx context.Context, request *openbaov1alpha1.OpenBaoRestore, action openbaov1alpha1.RestoreAdministratorDisposition) (string, error) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := m.reader.Get(ctx, client.ObjectKey{Namespace: request.Namespace, Name: request.Spec.Cluster}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			if action == openbaov1alpha1.RestoreAdministratorAbandon {
				return "", nil
			}
			return "Target is absent; use Abandon after administrator recovery", nil
		}
		return "", err
	}

	targetUID := cluster.UID
	if request.Status.Execution != nil && request.Status.Execution.TargetUID != "" {
		targetUID = request.Status.Execution.TargetUID
	} else if request.Status.Target != nil {
		targetUID = request.Status.Target.UID
	}
	if cluster.UID != targetUID {
		if action == openbaov1alpha1.RestoreAdministratorAbandon {
			return "", nil
		}
		return "Target UID differs; use Abandon after administrator recovery; the replacement will not be changed", nil
	}

	if action == openbaov1alpha1.RestoreAdministratorResume {
		if blocked, err := m.resumeAcknowledgedTarget(ctx, request, cluster); blocked != "" || err != nil {
			return blocked, err
		}
	}

	if cluster.Annotations[constants.AnnotationRestoreHold] == restoreExecutionOperationID(request) ||
		(action == openbaov1alpha1.RestoreAdministratorAbandon && cluster.Annotations[constants.AnnotationRestoreHold] == "" &&
			(cluster.Annotations[constants.AnnotationRestoreOrigin] == string(request.UID) || request.Status.Restart != nil)) {
		before := cluster.DeepCopy()
		if action == openbaov1alpha1.RestoreAdministratorAbandon {
			cluster.Spec.Paused = true
		}
		delete(cluster.Annotations, constants.AnnotationRestoreHold)
		if request.Status.Target != nil && cluster.Annotations[constants.AnnotationRestoreOrigin] == string(request.UID) {
			delete(cluster.Annotations, constants.AnnotationRestoreOrigin)
		}
		if err := m.client.Patch(ctx, cluster, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
			return "", err
		}
	}
	if err := opslifecycle.ReleaseWithReader(ctx, m.reader, m.client, cluster, restoreOperationLock(request)); err != nil && !opslifecycle.IsLockHeld(err) {
		return "", err
	}
	return "", nil
}

func (m *Manager) resumeAcknowledgedTarget(ctx context.Context, request *openbaov1alpha1.OpenBaoRestore, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.Paused || cluster.DeletionTimestamp != nil ||
		(cluster.Status.OperationLock != nil && !restoreOperationLock(request).IsHeldBy(cluster.Status.OperationLock)) {
		return "Target is paused, deleting, or locked by another operation; use Abandon for administrator recovery", nil
	}
	execution := request.Status.Execution
	if execution == nil || execution.JobUID == "" {
		return "Executor identity is unbound; use Abandon for administrator recovery", nil
	}
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: request.Namespace, Name: execution.JobName, UID: execution.JobUID}}
	pending, err := m.restoreExecutorPending(ctx, job)
	if err != nil {
		return "", err
	}
	if pending != "" {
		return "", &recoveryIssue{reason: ReasonRecoveryBlocked, message: pending}
	}
	done, err := m.restartAcknowledgedTarget(ctx, request, cluster)
	if err != nil {
		return "", err
	}
	if !done {
		return "", &recoveryIssue{reason: ReasonRecoveryRestarting,
			message: "Managed restart intent is recorded; the hold remains until all Pod replacements are ready"}
	}
	if hold := cluster.Annotations[constants.AnnotationRestoreHold]; hold != "" && hold != restoreExecutionOperationID(request) {
		return "Target has another restore hold; use Abandon for administrator recovery", nil
	}

	return "", nil
}
