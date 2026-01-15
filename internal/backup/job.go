package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/kube"
)

const (
	backupJobNamePrefix = "backup-"
	backupJobTTLSeconds = 3600 // 1 hour TTL for completed/failed jobs
)

const (
	awsWebIdentityVolumeName     = "aws-iam-token"
	awsWebIdentityMountPath      = "/var/run/secrets/aws"
	awsWebIdentityTokenFile      = "/var/run/secrets/aws/token" // #nosec G101 -- This is a mount path constant, not a credential
	awsWebIdentityTokenAudience  = "sts.amazonaws.com"          // #nosec G101 -- This is a constant, not a credential
	openBaoTokenVolumeName       = "openbao-token"
	openBaoTokenMountPath        = "/var/run/secrets/tokens" // #nosec G101 -- This is a mount path constant, not a credential
	openBaoTokenFileRelativePath = "openbao-token"
	openBaoTokenAudience         = "openbao-internal"
	backupCredentialsVolumeName  = "backup-credentials"          // #nosec G101 -- This is a volume name constant, not a credential
	backupCredentialsMountPath   = "/etc/bao/backup/credentials" // #nosec G101 -- This is a mount path constant, not a credential
	backupTokenVolumeName        = "backup-token"
	backupTokenMountPath         = "/etc/bao/backup/token" // #nosec G101 -- This is a mount path constant, not a credential
	backupTLSCAVolumeName        = "tls-ca"
)

// ensureBackupJob creates or updates a Kubernetes Job for executing the backup.
// Returns true if a Job was created or is already running, false if backup should not proceed.
func (m *Manager) ensureBackupJob(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string, scheduledTime time.Time) (bool, error) {
	job := &batchv1.Job{}

	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to get backup Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		// Job doesn't exist - create it
		logger.Info("Creating backup Job", "job", jobName)

		// Generate deterministic backup key
		// Format: <pathPrefix>/<namespace>/<cluster>/[<filenamePrefix>-]<timestamp>-<uuid>.snap
		// precise timestamp from schedule to match job name
		backupKey, err := GenerateBackupKey(
			cluster.Spec.Backup.Target.PathPrefix,
			cluster.Namespace,
			cluster.Name,
			"", // filenamePrefix (empty default)
			scheduledTime,
		)
		if err != nil {
			return false, fmt.Errorf("failed to generate backup key: %w", err)
		}

		verifiedExecutorDigest := ""
		executorImage, err := GetBackupExecutorImage(cluster)
		if err != nil {
			return false, fmt.Errorf("failed to get backup executor image: %w", err)
		}
		// Use OperatorImageVerification only - no fallback to ImageVerification
		verificationConfig := cluster.Spec.OperatorImageVerification
		if executorImage != "" && verificationConfig != nil && verificationConfig.Enabled {
			verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
			defer cancel()

			digest, err := security.VerifyOperatorImageForCluster(verifyCtx, logger, m.operatorImageVerifier, cluster, executorImage)
			if err != nil {
				failurePolicy := verificationConfig.FailurePolicy
				if failurePolicy == "" {
					failurePolicy = constants.ImageVerificationFailurePolicyBlock
				}
				if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
					if cluster.Status.Backup != nil {
						cluster.Status.Backup.LastFailureReason = fmt.Sprintf("%s: %v", constants.ReasonBackupExecutorImageVerificationFailed, err)
					}
					return false, fmt.Errorf("backup executor image verification failed (policy=Block): %w", err)
				}

				if cluster.Status.Backup != nil {
					cluster.Status.Backup.LastFailureReason = fmt.Sprintf("%s: %v", constants.ReasonBackupExecutorImageVerificationFailed, err)
				}
				logger.Error(err, "Backup executor image verification failed but proceeding due to Warn policy", "image", executorImage)
			} else {
				verifiedExecutorDigest = digest
				logger.Info("Backup executor image verified successfully", "digest", digest)
			}
		}

		job, buildErr := BuildJob(cluster, JobOptions{
			JobName:                jobName,
			JobType:                JobTypeScheduled,
			BackupKey:              backupKey,
			VerifiedExecutorDigest: verifiedExecutorDigest,
			ClientConfig:           m.clientConfig,
		})
		if buildErr != nil {
			return false, fmt.Errorf("failed to build backup Job %s/%s: %w", cluster.Namespace, jobName, buildErr)
		}

		// Set OwnerReference for garbage collection
		if err := controllerutil.SetControllerReference(cluster, job, m.scheme); err != nil {
			return false, fmt.Errorf("failed to set owner reference on backup Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		if err := m.client.Create(ctx, job); err != nil {
			return false, fmt.Errorf("failed to create backup Job %s/%s: %w", cluster.Namespace, jobName, err)
		}

		logger.Info("Backup Job created", "job", jobName, "backupKey", backupKey)
		return true, nil
	}

	// Job exists - check its status
	if kube.JobSucceeded(job) {
		logger.Info("Backup Job completed successfully", "job", jobName)
		return false, nil // Job completed, backup manager will process result
	}

	if kube.JobFailed(job) {
		// Check if we should retry (e.g., if it's a transient error)
		// For now, we'll let the Job be cleaned up and a new one created on next reconcile
		logger.Info("Backup Job failed", "job", jobName, "failed", job.Status.Failed)
		return false, nil
	}

	// Job is still running or pending
	logger.V(1).Info("Backup Job is in progress", "job", jobName,
		"active", job.Status.Active,
		"succeeded", job.Status.Succeeded,
		"failed", job.Status.Failed)
	return true, nil
}

// processBackupJobResult processes the result of a completed backup Job and updates cluster status.
// Returns (statusUpdated, error) where statusUpdated indicates if the status was modified
// (job completed successfully or failed). This is used to determine if a requeue is needed
// to persist the status update.
func (m *Manager) processBackupJobResult(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (bool, error) {
	job := &batchv1.Job{}

	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      jobName,
	}, job)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// Job was cleaned up or doesn't exist
			return false, nil
		}
		return false, fmt.Errorf("failed to get backup Job %s/%s: %w", cluster.Namespace, jobName, err)
	}

	// Check Job status
	if kube.JobSucceeded(job) {
		// Job succeeded - extract result from Job annotations
		// The backup key is stored in the Job annotation by the manager when creating the job
		backupKey := job.Annotations["openbao.org/backup-key"]

		now := metav1.Now()
		cluster.Status.Backup.LastBackupTime = &now
		if backupKey != "" {
			cluster.Status.Backup.LastBackupName = backupKey
		}
		cluster.Status.Backup.ConsecutiveFailures = 0
		cluster.Status.Backup.LastFailureReason = ""

		logger.Info("Backup Job completed successfully, status updated", "job", jobName, "lastBackupTime", now, "backupKey", backupKey)
		return true, nil // Status was updated - request requeue to persist
	}

	if kube.JobFailed(job) {
		// Job failed
		cluster.Status.Backup.ConsecutiveFailures++
		cluster.Status.Backup.LastFailureReason = fmt.Sprintf("Backup Job %s failed", jobName)

		logger.Error(fmt.Errorf("backup job failed"), "Backup Job failed, status updated",
			"job", jobName,
			"consecutiveFailures", cluster.Status.Backup.ConsecutiveFailures)
		return true, nil // Status was updated - request requeue to persist
	}

	// Job is still running
	return false, nil // Status updated but job still running - no requeue needed yet
}

func backupJobName(cluster *openbaov1alpha1.OpenBaoCluster, scheduledTime time.Time) string {
	return backupJobNamePrefix + cluster.Name + "-" + scheduledTime.UTC().Format("20060102-150405")
}

// NOTE: buildBackupJob, buildBackupJobVolumeMounts, buildBackupJobVolumes, and getBackupExecutorImage
// have been moved to job_builder.go as BuildJob, BuildBackupJobVolumeMounts, BuildBackupJobVolumes,
// and GetBackupExecutorImage respectively.
