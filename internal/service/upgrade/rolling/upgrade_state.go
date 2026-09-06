package rolling

import (
	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// detectUpgradeState determines whether an upgrade is needed or if we're resuming one.
func (m *Manager) detectUpgradeState(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, acknowledgements *upgrade.RequestAcknowledgements) (upgradeNeeded bool, resumeUpgrade bool) {
	if upgrade.RetryRequestPending(cluster) &&
		(cluster.Status.Upgrade == nil ||
			!upgrade.UpgradeFailed(cluster.Status.Upgrade) ||
			cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion) {
		retryRequest := upgrade.RetryRequestValue(cluster)
		acknowledgements.Retry = retryRequest
		logger.Info("Ignoring retry request because no failed rolling upgrade is waiting to resume",
			"retryRequest", retryRequest,
			"retryRequestField", upgrade.RequestRetryFieldPath)
	}

	if cluster.Status.Upgrade != nil {
		if upgrade.UpgradeFailed(cluster.Status.Upgrade) {
			failureReason := upgrade.UpgradeFailureReason(cluster.Status.Upgrade)
			failureMessage := upgrade.UpgradeFailureMessage(cluster.Status.Upgrade)
			if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
				logger.Info("Failed upgrade target differs from spec; resuming to re-evaluate upgrade target",
					"failedTargetVersion", cluster.Status.Upgrade.TargetVersion,
					"specVersion", cluster.Spec.Version,
					"failureReason", failureReason)
				return false, true
			}

			if !upgrade.RetryRequestPending(cluster) {
				logger.Info("Upgrade is in failed state; waiting for manual retry request",
					"failureReason", failureReason,
					"failureMessage", failureMessage,
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
