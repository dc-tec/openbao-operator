package upgrade

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	RequestRetryFieldPath    = "spec.upgrade.requests.retry"
	RequestPromoteFieldPath  = "spec.upgrade.requests.promote"
	RequestRollbackFieldPath = "spec.upgrade.requests.rollback"
)

func RetryRequestValue(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Requests == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Spec.Upgrade.Requests.Retry)
}

func PromoteRequestValue(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Requests == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Spec.Upgrade.Requests.Promote)
}

func RollbackRequestValue(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Requests == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Spec.Upgrade.Requests.Rollback)
}

func RetryRequestPending(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	request := RetryRequestValue(cluster)
	return request != "" && request != lastHandledRetry(cluster)
}

func PromoteRequestPending(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	request := PromoteRequestValue(cluster)
	return request != "" && request != lastHandledPromote(cluster)
}

func RollbackRequestPending(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	request := RollbackRequestValue(cluster)
	return request != "" && request != lastHandledRollback(cluster)
}

func MarkRetryRequestHandled(status *openbaov1alpha1.OpenBaoClusterStatus, value string) {
	if status == nil {
		return
	}
	ensureUpgradeRequestStatus(status).LastHandledRetry = strings.TrimSpace(value)
}

func MarkPromoteRequestHandled(status *openbaov1alpha1.OpenBaoClusterStatus, value string) {
	if status == nil {
		return
	}
	ensureUpgradeRequestStatus(status).LastHandledPromote = strings.TrimSpace(value)
}

func MarkRollbackRequestHandled(status *openbaov1alpha1.OpenBaoClusterStatus, value string) {
	if status == nil {
		return
	}
	ensureUpgradeRequestStatus(status).LastHandledRollback = strings.TrimSpace(value)
}

func ensureUpgradeRequestStatus(status *openbaov1alpha1.OpenBaoClusterStatus) *openbaov1alpha1.UpgradeRequestStatus {
	if status.UpgradeRequests == nil {
		status.UpgradeRequests = &openbaov1alpha1.UpgradeRequestStatus{}
	}
	return status.UpgradeRequests
}

func lastHandledRetry(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Status.UpgradeRequests == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Status.UpgradeRequests.LastHandledRetry)
}

func lastHandledPromote(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Status.UpgradeRequests == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Status.UpgradeRequests.LastHandledPromote)
}

func lastHandledRollback(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Status.UpgradeRequests == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Status.UpgradeRequests.LastHandledRollback)
}
