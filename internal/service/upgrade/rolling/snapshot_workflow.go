package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/snapshot"
)

// handlePreUpgradeSnapshot checks if preUpgradeSnapshot is enabled and triggers a backup if needed.
// Returns true if the snapshot is complete (or disabled), false if it is in progress (created or running).
// Returns an error if backup fails, which will block the upgrade.
func (m *Manager) handlePreUpgradeSnapshot(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	if cluster.Spec.Upgrade == nil || !cluster.Spec.Upgrade.PreUpgradeSnapshot {
		logger.V(1).Info("Pre-upgrade snapshot is not enabled")
		return true, nil
	}

	if err := m.validatePreUpgradeSnapshotPrerequisites(ctx, cluster); err != nil {
		return false, err
	}

	existingJobName, existingJobStatus, err := m.findExistingPreUpgradeBackupJob(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to check for existing pre-upgrade backup job: %w", err)
	}
	if existingJobName != "" {
		return m.reconcileExistingPreUpgradeBackupJob(ctx, logger, cluster, existingJobName, existingJobStatus)
	}

	return m.createPreUpgradeBackupJob(ctx, logger, cluster)
}

func (m *Manager) validatePreUpgradeSnapshotPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return snapshot.ValidatePreUpgradeSnapshotPrerequisites(ctx, m.client, cluster, snapshot.ValidationOptions{
		MissingBackupMessage:  "backup configuration is required when preUpgradeSnapshot is enabled",
		RequireEndpoint:       false,
		RequireTokenSecret:    true,
		NetworkErrorMessage:   "hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so backup Jobs can reach the object storage endpoint",
		AuthenticationMessage: "backup authentication is required: either jwtAuthRole or tokenSecretRef must be set",
	})
}
