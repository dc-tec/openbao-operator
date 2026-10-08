package restore

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/constants"
)

const (
	ReasonRecoveryAwaitingAcknowledgement = "AwaitingAcknowledgement"
	ReasonRecoveryBlocked                 = "RecoveryBlocked"
	ReasonRecoveryWaitingForPod           = "WaitingForPod"
	ReasonRecoveryHealthUnavailable       = "VoterHealthUnavailable"
	ReasonRecoveryAccessUnavailable       = "OperatorAccessUnavailable"
	ReasonRecoveryMembershipChanged       = "MembershipChanged"
	ReasonRecoveryRestarting              = "Restarting"
	ReasonRecoveryResumed                 = "Resumed"
	ReasonRecoveryAbandoned               = "Abandoned"
)

// recoveryIssue carries an operator-defined diagnostic. Provider responses stay
// in the error chain and are never copied into request status.
type recoveryIssue struct {
	reason  string
	message string
	cause   error
}

func (e *recoveryIssue) Error() string { return e.message }
func (e *recoveryIssue) Unwrap() error { return e.cause }

func (m *Manager) reportRecovery(ctx context.Context, request *api.OpenBaoRestore, reason, message string) (ctrl.Result, error) {
	current := meta.FindStatusCondition(request.Status.Conditions, constants.RestoreRecoveryReleasedConditionType)
	if request.Status.Message == message && current != nil && current.Status == metav1.ConditionFalse &&
		current.Reason == reason && current.Message == message && current.ObservedGeneration == request.Generation {
		return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, nil
	}

	before := request.DeepCopy()
	request.Status.Message = message
	meta.SetStatusCondition(&request.Status.Conditions, metav1.Condition{
		Type: constants.RestoreRecoveryReleasedConditionType, Status: metav1.ConditionFalse,
		Reason: reason, Message: message, ObservedGeneration: request.Generation,
	})
	return ctrl.Result{RequeueAfter: restoreRequeueJobCheck}, m.patchStatus(ctx, request, before)
}
