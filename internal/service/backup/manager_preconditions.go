package backup

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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

	// Check we have a token for backup.
	// All clusters (both standard and self-init) must use either JWT Auth
	// or a backup token Secret. Root tokens are not used for security reasons.
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return newBackupWarningError(ReasonNoBackupToken, ErrNoBackupToken.Error())
	}

	// Check if JWT Auth is configured (preferred method).
	// If jwtAuthRole is empty, check if OIDC is enabled (operator will auto-create the backup role).
	hasJWTAuth := strings.TrimSpace(backupCfg.JWTAuthRole) != ""
	if !hasJWTAuth && cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.OIDC != nil && cluster.Spec.SelfInit.OIDC.Enabled {
		// Operator will auto-create the backup role with name auth.RoleNameBackup.
		hasJWTAuth = true
	}

	// Check if static token is configured (fallback method).
	hasTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""

	// At least one authentication method must be configured.
	if !hasJWTAuth && !hasTokenSecret {
		return newBackupWarningError(ReasonNoBackupToken, ErrNoBackupToken.Error())
	}

	// If using token secret, verify it exists.
	// SECURITY: Always use cluster.Namespace for secret lookups.
	// Do NOT trust user-provided Namespace in SecretRef to prevent Confused Deputy attacks.
	if hasTokenSecret {
		secretNamespace := cluster.Namespace

		secretName := types.NamespacedName{
			Namespace: secretNamespace,
			Name:      backupCfg.TokenSecretRef.Name,
		}

		secret := &corev1.Secret{}
		if err := m.client.Get(ctx, secretName, secret); err != nil {
			if apierrors.IsNotFound(err) {
				return newBackupWarningError(ReasonNoBackupToken, fmt.Sprintf("backup token Secret %s/%s not found: %v", secretNamespace, backupCfg.TokenSecretRef.Name, ErrNoBackupToken))
			}
			return fmt.Errorf("failed to get backup token Secret %s/%s: %w", secretNamespace, backupCfg.TokenSecretRef.Name, err)
		}
	}

	return nil
}
