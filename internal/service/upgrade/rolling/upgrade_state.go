package rolling

import (
	"strings"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// detectUpgradeState determines whether an upgrade is needed or if we're resuming one.
func (m *Manager) detectUpgradeState(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (upgradeNeeded bool, resumeUpgrade bool) {
	if upgrade.RetryRequestPending(cluster) &&
		(cluster.Status.Upgrade == nil ||
			strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason) == "" ||
			cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion) {
		retryRequest := upgrade.RetryRequestValue(cluster)
		upgrade.MarkRetryRequestHandled(&cluster.Status, retryRequest)
		logger.Info("Ignoring retry request because no failed rolling upgrade is waiting to resume",
			"retryRequest", retryRequest,
			"retryRequestField", upgrade.RequestRetryFieldPath)
	}

	if cluster.Status.Upgrade != nil {
		if strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason) != "" {
			if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
				logger.Info("Failed upgrade target differs from spec; resuming to re-evaluate upgrade target",
					"failedTargetVersion", cluster.Status.Upgrade.TargetVersion,
					"specVersion", cluster.Spec.Version,
					"failureReason", cluster.Status.Upgrade.LastErrorReason)
				return false, true
			}

			if !upgrade.RetryRequestPending(cluster) {
				logger.Info("Upgrade is in failed state; waiting for manual retry request",
					"failureReason", cluster.Status.Upgrade.LastErrorReason,
					"failureMessage", cluster.Status.Upgrade.LastErrorMessage,
					"retryRequestField", upgrade.RequestRetryFieldPath)
				return false, false
			}

			logger.Info("Manual retry requested for failed upgrade",
				"retryRequest", upgrade.RetryRequestValue(cluster),
				"targetVersion", cluster.Status.Upgrade.TargetVersion,
				"currentPartition", cluster.Status.Upgrade.CurrentPartition)
			return false, true
		}

		logger.Info("Resuming in-progress upgrade",
			"fromVersion", cluster.Status.Upgrade.FromVersion,
			"targetVersion", cluster.Status.Upgrade.TargetVersion,
			"currentPartition", cluster.Status.Upgrade.CurrentPartition)
		return false, true
	}

	if cluster.Status.CurrentVersion == "" {
		logger.Info("Setting initial CurrentVersion from spec", "version", cluster.Spec.Version)
		return false, false
	}

	if cluster.Spec.Version == cluster.Status.CurrentVersion {
		logger.V(1).Info("No upgrade needed; versions match")
		return false, false
	}

	logger.Info("Upgrade detected",
		"from", cluster.Status.CurrentVersion,
		"to", cluster.Spec.Version)
	return true, false
}
