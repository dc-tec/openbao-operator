package rolling

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// validateUpgrade performs pre-upgrade validation checks.
func (m *Manager) validateUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := upgrade.ValidateUpgradeTargetVersion(logger, cluster.Status.CurrentVersion, cluster.Spec.Version); err != nil {
		return err
	}
	if err := upgrade.ValidateImageRefMatchesVersion(cluster.Spec.Version, cluster.Spec.Image); err != nil {
		return err
	}

	// New upgrades require a fully healthy cluster. In-progress upgrades use a
	// narrower gate so the target pod can be temporarily unavailable while the
	// controller waits for it to recover or time out.
	if cluster.Status.Upgrade != nil {
		if err := m.verifyResumeClusterHealth(ctx, logger, cluster); err != nil {
			return err
		}
		return nil
	}

	if err := m.verifyClusterHealth(ctx, logger, cluster); err != nil {
		return err
	}

	return nil
}
