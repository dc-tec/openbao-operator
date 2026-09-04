package backup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
)

func manualTriggerToken(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	return cluster.Annotations[constants.AnnotationTriggerBackup]
}

func (m *Manager) recordManualTriggerAccepted(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, token string) {
	recordManualTriggerDetected(logger, cluster, token)
	m.emitNormalEvent(cluster, ReasonBackupManualTriggerAccepted, "Accepted manual backup trigger %q", token)
}

func recordManualTriggerDetected(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, token string) {
	logger.Info("Manual backup trigger detected", "annotation", token)
	logging.LogAuditEvent(logger, logging.EventBackupManualTriggerDetected, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"trigger":           "manual_annotation",
	})
}

func (m *Manager) skipManualTriggerForActiveJob(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	token string,
) error {
	recordManualTriggerDetected(logger, cluster, token)
	logger.Info("Manual backup triggered but job already in progress, skipping duplicate")
	logging.LogAuditEvent(logger, logging.EventBackupManualTriggerSkipped, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"reason":            "active_job_in_progress",
	})
	m.emitNormalEvent(cluster, ReasonBackupSkipped, "Skipping manual backup because a backup Job is already in progress")
	return m.clearManualTriggerAnnotation(ctx, logger, cluster)
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
