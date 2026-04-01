package rolling

import (
	"context"
	"strings"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

type upgradeFailureDetails struct {
	FirstFailure bool
	Reason       string
	Message      string
}

func resolveUpgradeFailure(cluster *openbaov1alpha1.OpenBaoCluster, defaultReason string, defaultMessage string) upgradeFailureDetails {
	details := upgradeFailureDetails{
		FirstFailure: true,
		Reason:       defaultReason,
		Message:      defaultMessage,
	}
	if cluster == nil {
		return details
	}

	if cluster.Status.Upgrade != nil {
		details.FirstFailure = cluster.Status.Upgrade.LastErrorAt == nil
	}

	if cluster.Status.Upgrade == nil || strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason) == "" {
		core.SetUpgradeFailed(&cluster.Status, defaultReason, defaultMessage)
	}

	if cluster.Status.Upgrade != nil {
		if reason := strings.TrimSpace(cluster.Status.Upgrade.LastErrorReason); reason != "" {
			details.Reason = reason
		}
		if message := strings.TrimSpace(cluster.Status.Upgrade.LastErrorMessage); message != "" {
			details.Message = message
		}
	}

	return details
}

func (m *Manager) recordUpgradeFailure(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *upgrade.Metrics,
	strategy string,
	defaultReason string,
	defaultMessage string,
	patchErrorLog string,
) {
	details := resolveUpgradeFailure(cluster, defaultReason, defaultMessage)

	if metrics != nil {
		metrics.SetStatus(upgrade.UpgradeStatusFailed)
	}

	if details.FirstFailure {
		if metrics != nil {
			metrics.IncrementFailure(strategy)
		}
		logging.LogAuditEvent(logger, logging.EventUpgradeFailed, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"strategy":          strategy,
			"reason":            details.Reason,
		})
		m.emitWarningEvent(cluster, details.Reason, upgrade.MessageUpgradeFailed, details.Message)
	}

	if err := m.patchUpgradeStatus(ctx, cluster); err != nil {
		logger.Error(err, patchErrorLog)
	}
}

func (m *Manager) patchUpgradeStatus(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return m.patchStatusSSA(ctx, cluster)
}
