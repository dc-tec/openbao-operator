package backup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/workloadidentity"
)

type backupPreconditionError struct {
	reason    string
	message   string
	eventType string
}

func (e *backupPreconditionError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func newBackupSkipError(message string) error {
	return &backupPreconditionError{
		reason:    ReasonBackupSkipped,
		message:   message,
		eventType: corev1.EventTypeNormal,
	}
}

func newBackupWarningError(reason, message string) error {
	return &backupPreconditionError{
		reason:    reason,
		message:   message,
		eventType: corev1.EventTypeWarning,
	}
}

func (m *Manager) hasInProgressRestore(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	restoreList := &openbaov1alpha1.OpenBaoRestoreList{}
	if err := m.client.List(ctx, restoreList, client.InNamespace(cluster.Namespace)); err != nil {
		// In some zero-trust deployments, the OpenBaoCluster controller may not have permissions
		// to list restore resources. Fallback to "no restore detected" in that case.
		if apierrors.IsForbidden(err) {
			logger.V(1).Info("Insufficient permissions to list OpenBaoRestore resources; cannot detect restore-in-progress", "error", err.Error())
			return false, nil
		}
		return false, fmt.Errorf("failed to list OpenBaoRestore resources: %w", err)
	}

	for i := range restoreList.Items {
		restore := &restoreList.Items[i]
		if restore.DeletionTimestamp != nil {
			continue
		}
		if restore.Spec.Cluster != cluster.Name {
			continue
		}
		if restore.Status.Phase == openbaov1alpha1.RestorePhaseCompleted ||
			restore.Status.Phase == openbaov1alpha1.RestorePhaseFailed {
			continue
		}
		return true, nil
	}

	return false, nil
}

// checkPreconditions verifies that backup can proceed.
func (m *Manager) checkPreconditions(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Check cluster is initialized.
	if !cluster.Status.Initialized {
		return newBackupSkipError("cluster is not initialized")
	}

	// Check cluster phase - don't backup during initialization.
	if cluster.Status.Phase == openbaov1alpha1.ClusterPhaseInitializing {
		return newBackupSkipError("cluster is initializing")
	}

	// Check if an upgrade is about to start or in progress.
	// This prevents regular backups from starting when an upgrade is detected or in progress.
	// We use the same logic as the upgrade manager to detect pending upgrades.
	// This catches upgrades before Status.Upgrade is set and before pre-upgrade jobs are visible.
	if cluster.Status.Initialized {
		// Only check for pending upgrades if cluster is initialized
		// (upgrade manager also skips if not initialized).
		if cluster.Status.CurrentVersion != "" {
			// CurrentVersion is set - check if it differs from spec.
			if cluster.Spec.Version != cluster.Status.CurrentVersion {
				// Upgrade is about to start - check if pre-upgrade snapshot is enabled.
				if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.PreUpgradeSnapshot {
					// Pre-upgrade snapshot is enabled - skip regular backups.
					// The upgrade manager will handle the pre-upgrade backup.
					return newBackupSkipError("upgrade pending with pre-upgrade snapshot enabled")
				}
				// Upgrade is about to start but no pre-upgrade snapshot - still skip regular backups.
				return newBackupSkipError("upgrade pending")
			}
		}
		// If CurrentVersion is empty but cluster is initialized, this is the first reconcile after init.
		// The upgrade manager will set CurrentVersion, so no upgrade is pending yet.
	}

	// Check if upgrade is in progress - skip scheduled backups during upgrades.
	// Exception: Pre-upgrade backups are triggered by the upgrade manager, not here.
	if cluster.Status.Upgrade != nil {
		return newBackupSkipError("upgrade in progress")
	}

	// Check if a pre-upgrade backup job exists or is in progress.
	// This is a fallback check in case the version check above didn't catch it
	// (e.g., if Status.CurrentVersion is empty or there's a timing issue).
	hasPreUpgradeJob, err := m.hasPreUpgradeBackupJob(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to check for pre-upgrade backup job: %w", err)
	}
	if hasPreUpgradeJob {
		return newBackupSkipError("pre-upgrade backup in progress")
	}

	readiness, err := workloadidentity.EvaluateBackupReadiness(ctx, m.client, cluster)
	if err != nil {
		return fmt.Errorf("failed to evaluate backup Job prerequisites: %w", err)
	}
	if readiness.Status != metav1.ConditionTrue {
		return newBackupWarningError(readiness.Reason, readiness.Message)
	}

	return nil
}
