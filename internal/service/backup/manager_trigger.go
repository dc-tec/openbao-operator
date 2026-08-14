package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
)

// handleManualTrigger checks for and handles manual backup trigger annotation.
// Returns (manualTriggerToken, scheduledTime, error).
func (m *Manager) handleManualTrigger(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	now time.Time,
) (string, time.Time, error) {
	triggerAnnotation := constants.AnnotationTriggerBackup
	val, ok := cluster.Annotations[triggerAnnotation]
	if !ok || val == "" {
		return "", time.Time{}, nil
	}

	logger.Info("Manual backup trigger detected", "annotation", val)
	logging.LogAuditEvent(logger, logging.EventBackupManualTriggerDetected, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"trigger":           "manual_annotation",
	})

	// Check if there's already a backup job in progress.
	hasActiveJob, err := m.hasActiveBackupJob(ctx, cluster)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasActiveJob {
		logger.Info("Manual backup triggered but job already in progress, skipping duplicate")
		logging.LogAuditEvent(logger, logging.EventBackupManualTriggerSkipped, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"reason":            "active_job_in_progress",
		})
		m.emitNormalEvent(cluster, ReasonBackupSkipped, "Skipping manual backup because a backup Job is already in progress")
		if err := m.clearManualTriggerAnnotation(ctx, logger, cluster); err != nil {
			return "", time.Time{}, err
		}
		return "", time.Time{}, nil
	}

	m.emitNormalEvent(cluster, ReasonBackupManualTriggerAccepted, "Accepted manual backup trigger %q", val)

	return val, now, nil
}

// clearManualTriggerAnnotation removes the manual backup trigger annotation from the cluster.
func (m *Manager) clearManualTriggerAnnotation(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	const annotation = constants.AnnotationTriggerBackup
	if cluster.Annotations == nil {
		return nil
	}
	if _, found := cluster.Annotations[annotation]; !found {
		return nil
	}

	// We use MergeFrom for annotation deletion to avoid claiming ownership of all annotations.
	original := cluster.DeepCopy()
	delete(cluster.Annotations, annotation)
	if err := m.client.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to clear manual backup trigger annotation: %w", err)
	}
	logger.Info("Cleared manual backup trigger annotation")
	return nil
}
