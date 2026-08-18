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
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
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

	if err := m.reader.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return false, fmt.Errorf("failed to list pre-upgrade backup jobs: %w", err)
	}

	// Check if there's a running or pending job (not yet succeeded or failed).
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if err := opslifecycle.RequireManagedJobOwner(
			"observe pre-upgrade backup",
			job,
			cluster,
			openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
		); err != nil {
			return false, err
		}
		// If job hasn't succeeded or failed, it's still running or pending.
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			return true, nil
		}
	}

	return false, nil
}

type backupJobObservation struct {
	hasActive          bool
	mostRecentTerminal *batchv1.Job
}

// observeBackupJobs reads active and terminal Job state from one live API snapshot.
func (m *Manager) observeBackupJobs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (backupJobObservation, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	})

	if err := m.reader.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return backupJobObservation{}, fmt.Errorf("failed to list backup jobs: %w", err)
	}

	observation := backupJobObservation{}
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if err := opslifecycle.RequireManagedJobOwner(
			"observe backup",
			job,
			cluster,
			openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
		); err != nil {
			return backupJobObservation{}, err
		}
		if !kube.JobSucceeded(job) && !kube.JobFailed(job) {
			observation.hasActive = true
			continue
		}
		if observation.mostRecentTerminal == nil || job.CreationTimestamp.After(observation.mostRecentTerminal.CreationTimestamp.Time) {
			observation.mostRecentTerminal = job.DeepCopy()
		}
	}
	return observation, nil
}

// checkForCompletedJobs checks for any completed backup jobs and processes them.
// This is used to ensure completed jobs are processed even when backup is not due yet.
// Only the most recent completed job is processed to avoid incrementing ConsecutiveFailures multiple times
// when there are several old failed jobs.
func (m *Manager) checkForCompletedJobs(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (backupJobProcessResult, error) {
	observation, err := m.observeBackupJobs(ctx, cluster)
	if err != nil {
		return backupJobProcessResult{}, err
	}

	if observation.mostRecentTerminal == nil {
		return backupJobProcessResult{}, nil
	}

	job := observation.mostRecentTerminal
	logger.Info("Processing completed backup job", "job", job.Name,
		"succeeded", job.Status.Succeeded, "failed", job.Status.Failed)

	result, err := m.processBackupJob(ctx, logger, cluster, job)
	if err != nil {
		return backupJobProcessResult{}, err
	}
	if result.statusUpdated {
		logger.Info("Completed backup job processed, status updated", "job", job.Name)
	} else {
		logger.V(1).Info("Completed backup job already processed", "job", job.Name)
	}

	return result, nil
}

// hasActiveBackupJob checks if there's any backup job (scheduled or manual) running or pending for this cluster.
// This is used to prevent duplicate jobs from being created when manual triggers are processed multiple times.
func (m *Manager) hasActiveBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	observation, err := m.observeBackupJobs(ctx, cluster)
	return observation.hasActive, err
}
