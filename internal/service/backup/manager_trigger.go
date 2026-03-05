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
// Returns (manualTrigger, scheduledTime, error).
func (m *Manager) handleManualTrigger(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	now time.Time,
) (bool, time.Time, error) {
	triggerAnnotation := constants.AnnotationTriggerBackup
	val, ok := cluster.Annotations[triggerAnnotation]
	if !ok || val == "" {
		return false, time.Time{}, nil
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
		return false, time.Time{}, fmt.Errorf("failed to check for active backup job: %w", err)
	}
	if hasActiveJob {
		logger.Info("Manual backup triggered but job already in progress, skipping duplicate")
		logging.LogAuditEvent(logger, logging.EventBackupManualTriggerSkipped, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"reason":            "active_job_in_progress",
		})
		m.clearTriggerAnnotation(ctx, logger, cluster, triggerAnnotation)
		return false, time.Time{}, nil
	}

	return true, now, nil
}

// clearTriggerAnnotation removes the manual trigger annotation from the cluster.
func (m *Manager) clearTriggerAnnotation(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, annotation string) {
	// We use MergeFrom for annotation deletion to avoid claiming ownership of all annotations.
	original := cluster.DeepCopy()
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	delete(cluster.Annotations, annotation)
	if err := m.client.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		logger.Error(err, "Failed to clear manual backup trigger annotation")
	} else {
		logger.Info("Cleared manual backup trigger annotation")
	}
}
