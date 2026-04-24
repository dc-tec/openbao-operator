package openbaoclusterclaim

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const reasonBackupFailed = "BackupFailed"

func desiredBackupStatus(localCluster *openbaov1alpha1.OpenBaoCluster) *openbaov1alpha1.OpenBaoClusterClaimBackupStatus {
	return desiredBackupStatusWithRequest(localCluster, nil)
}

func desiredBackupStatusWithRequest(
	localCluster *openbaov1alpha1.OpenBaoCluster,
	request *openbaov1alpha1.OpenBaoClusterClaimBackupRequest,
) *openbaov1alpha1.OpenBaoClusterClaimBackupStatus {
	if localCluster == nil && request == nil {
		return nil
	}

	status := &openbaov1alpha1.OpenBaoClusterClaimBackupStatus{}
	if localCluster != nil {
		backup := localCluster.Status.Backup
		status.InProgress = localClusterBackupInProgress(localCluster)
		if backup != nil {
			status.LastBackupTime = backup.LastBackupTime.DeepCopy()
			status.LastBackupName = backup.LastBackupName
			status.LastAttemptTime = backup.LastAttemptTime.DeepCopy()
			status.NextScheduledBackup = backup.NextScheduledBackup.DeepCopy()
			status.LastBackupDuration = backup.LastBackupDuration
			status.ConsecutiveFailures = backup.ConsecutiveFailures
			status.LastFailureReason = backup.LastFailureReason
			status.LastFailureMessage = backup.LastFailureMessage
		}
	}
	if request != nil {
		state := request.Status.State
		if state == "" {
			state = openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending
		}
		status.RequestRef = &openbaov1alpha1.LocalReference{Name: request.Name}
		status.RequestState = state
		status.RequestReason = request.Status.Reason
	}

	if status.RequestRef != nil ||
		status.InProgress ||
		status.LastBackupTime != nil ||
		strings.TrimSpace(status.LastBackupName) != "" ||
		status.LastAttemptTime != nil ||
		status.NextScheduledBackup != nil ||
		strings.TrimSpace(status.LastBackupDuration) != "" ||
		status.ConsecutiveFailures > 0 ||
		strings.TrimSpace(status.LastFailureReason) != "" ||
		strings.TrimSpace(status.LastFailureMessage) != "" {
		return status
	}

	return nil
}

func localClusterBackupInProgress(localCluster *openbaov1alpha1.OpenBaoCluster) bool {
	if localCluster == nil {
		return false
	}
	if localCluster.Status.Phase == openbaov1alpha1.ClusterPhaseBackingUp {
		return true
	}
	condition := meta.FindStatusCondition(localCluster.Status.Conditions, string(openbaov1alpha1.ConditionBackingUp))
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func localClusterBackupFailure(localCluster *openbaov1alpha1.OpenBaoCluster) (string, string, bool) {
	if localCluster == nil || localCluster.Status.Backup == nil {
		return "", "", false
	}

	backup := localCluster.Status.Backup
	if backup.ConsecutiveFailures == 0 &&
		strings.TrimSpace(backup.LastFailureReason) == "" &&
		strings.TrimSpace(backup.LastFailureMessage) == "" {
		return "", "", false
	}

	reason := strings.TrimSpace(backup.LastFailureReason)
	if reason == "" {
		reason = reasonBackupFailed
	}
	message := strings.TrimSpace(backup.LastFailureMessage)
	if message == "" {
		message = "Service instance remains available, but backup automation is currently failing."
	}
	return reason, message, true
}

func localClusterBackupDegraded(localCluster *openbaov1alpha1.OpenBaoCluster) bool {
	if localClusterBackupInProgress(localCluster) {
		return true
	}
	_, _, ok := localClusterBackupFailure(localCluster)
	return ok
}
