package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

// Reconcile ensures backup configuration and status are aligned with the desired state for the given OpenBaoCluster.
// It checks if a backup is due, executes it if needed, and applies retention policies.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	logger = logger.WithValues("component", constants.ComponentBackup)
	metrics := NewMetrics(cluster.Namespace, cluster.Name)
	now := time.Now().UTC()

	if backupOperationLock.IsHeldBy(cluster.Status.OperationLock) {
		if err := m.ensureBackupStatus(ctx, cluster); err != nil {
			return recon.Result{}, err
		}
		m.syncBackupStatusMetrics(cluster, metrics)
	} else if cluster.Spec.Backup != nil {
		if err := validateBackupHardenedConfiguration(cluster); err != nil {
			return recon.Result{}, err
		}
		if err := m.syncBackupMetrics(ctx, logger, cluster, metrics); err != nil {
			return recon.Result{}, err
		}
		if err := m.ensureBackupServiceAccount(ctx, logger, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err)
		}
		if err := m.ensureBackupRBAC(ctx, logger, cluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to ensure backup RBAC: %w", err)
		}
		if err := m.ensureBackupStatus(ctx, cluster); err != nil {
			return recon.Result{}, err
		}
	}

	observation, err := m.observeBackup(ctx, logger, cluster, now)
	if err != nil {
		return recon.Result{}, err
	}
	return m.applyBackupDecision(ctx, logger, cluster, metrics, decideBackup(observation))
}

func (m *Manager) ensureBackupStatus(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.Backup != nil {
		return nil
	}

	cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to initialize backup status: %w", err)
	}
	return nil
}

func (m *Manager) releaseBackupLock(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, contextNote string) error {
	if err := opslifecycle.ReleaseWithReader(ctx, m.reader, m.client, cluster, backupOperationLock); err != nil && !opslifecycle.IsLockHeld(err) {
		logger.Error(err, "Failed to release backup operation lock", "context", contextNote)
		return fmt.Errorf("failed to release backup operation lock %s: %w", contextNote, err)
	} else if err == nil {
		logging.LogAuditEvent(logger, logging.EventOperationLockReleased, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"operation":         string(openbaov1alpha1.ClusterOperationBackup),
			"holder":            backupOperationLockHolder,
		})
	}
	return nil
}
