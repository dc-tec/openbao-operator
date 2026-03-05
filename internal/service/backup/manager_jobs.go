package backup

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// hasPreUpgradeBackupJob checks if there's a pre-upgrade backup job running or pending for this cluster.
// This is used to prevent regular scheduled backups from starting when an upgrade is initiating.
func (m *Manager) hasPreUpgradeBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:       cluster.Name,
		constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:    cluster.Name,
		constants.LabelOpenBaoComponent:  ComponentBackup,
		constants.LabelOpenBaoBackupType: "pre-upgrade",
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list pre-upgrade backup jobs: %w", err)
	}

	// Check if there's a running or pending job (not yet succeeded or failed).
	for i := range jobList.Items {
		job := &jobList.Items[i]
		// If job hasn't succeeded or failed, it's still running or pending.
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			return true, nil
		}
	}

	return false, nil
}

// checkForCompletedJobs checks for any completed backup jobs and processes them.
// Returns (statusUpdated, error) where statusUpdated indicates if any job was processed and status was updated.
// This is used to ensure completed jobs are processed even when backup is not due yet.
// Only the most recent completed job is processed to avoid incrementing ConsecutiveFailures multiple times
// when there are several old failed jobs.
func (m *Manager) checkForCompletedJobs(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list backup jobs: %w", err)
	}

	// Find the most recent completed job (by creation timestamp).
	// We only process the most recent one to avoid processing stale failures repeatedly.
	var mostRecentCompleted *batchv1.Job
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			continue // Skip jobs that are still running.
		}
		if mostRecentCompleted == nil || job.CreationTimestamp.After(mostRecentCompleted.CreationTimestamp.Time) {
			mostRecentCompleted = job
		}
	}

	if mostRecentCompleted == nil {
		return false, nil // No completed jobs to process.
	}

	logger.Info("Processing completed backup job", "job", mostRecentCompleted.Name,
		"succeeded", mostRecentCompleted.Status.Succeeded, "failed", mostRecentCompleted.Status.Failed)

	statusUpdated, err := m.processBackupJobResult(ctx, logger, cluster, mostRecentCompleted.Name)
	if err != nil {
		return false, err
	}
	if statusUpdated {
		logger.Info("Completed backup job processed, status updated", "job", mostRecentCompleted.Name)
	} else {
		logger.V(1).Info("Completed backup job already processed", "job", mostRecentCompleted.Name)
	}

	return statusUpdated, nil
}

// hasActiveBackupJob checks if there's any backup job (scheduled or manual) running or pending for this cluster.
// This is used to prevent duplicate jobs from being created when manual triggers are processed multiple times.
func (m *Manager) hasActiveBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list backup jobs: %w", err)
	}

	// Check if there's a running or pending job (not yet succeeded or failed).
	for i := range jobList.Items {
		job := &jobList.Items[i]
		// If job hasn't succeeded or failed, it's still running or pending.
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			return true, nil
		}
	}

	return false, nil
}
