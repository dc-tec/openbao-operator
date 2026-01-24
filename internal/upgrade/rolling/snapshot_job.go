package rolling

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/kube"
	"github.com/dc-tec/openbao-operator/internal/security"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// handlePreUpgradeSnapshot checks if preUpgradeSnapshot is enabled and triggers a backup if needed.
// Returns true if the snapshot is complete (or disabled), false if it is in progress (created or running).
// Returns an error if backup fails, which will block the upgrade.
func (m *Manager) handlePreUpgradeSnapshot(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	// Check if pre-upgrade snapshot is enabled
	if cluster.Spec.Upgrade == nil || !cluster.Spec.Upgrade.PreUpgradeSnapshot {
		logger.V(1).Info("Pre-upgrade snapshot is not enabled")
		return true, nil
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		return false, operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with pre-upgrade snapshots enabled requires explicit spec.network.egressRules so backup Jobs can reach the object storage endpoint",
			)),
		)
	}

	// Verify backup configuration is valid
	if err := m.validateBackupConfig(ctx, cluster); err != nil {
		return false, operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, fmt.Errorf("pre-upgrade backup configuration invalid: %w", err))
	}

	// Check if there's an existing pre-upgrade backup job (running or failed)
	existingJobName, existingJobStatus, err := m.findExistingPreUpgradeBackupJob(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to check for existing pre-upgrade backup job: %w", err)
	}

	var jobName string
	if existingJobName != "" {
		// Job exists - check its status
		jobName = existingJobName
		logger.Info("Found existing pre-upgrade backup job, checking status", "job", jobName)

		// If job failed, implement retry logic
		if existingJobStatus == "failed" {
			// Count how many failed jobs exist for this cluster
			failedCount, err := m.countFailedPreUpgradeBackupJobs(ctx, cluster)
			if err != nil {
				return false, fmt.Errorf("failed to count failed backup jobs: %w", err)
			}

			maxRetries := upgrade.DefaultMaxPreUpgradeBackupRetries
			if failedCount >= maxRetries {
				return false, operatorerrors.WithReason(
					upgrade.ReasonPreUpgradeBackupFailed,
					fmt.Errorf("pre-upgrade backup failed after %d attempts (max retries exceeded); manual intervention required", failedCount),
				)
			}

			// Delete the failed job to allow retry
			logger.Info("Deleting failed pre-upgrade backup job for retry",
				"job", jobName,
				"attempt", failedCount+1,
				"maxRetries", maxRetries)
			if err := m.deletePreUpgradeBackupJob(ctx, jobName, cluster.Namespace); err != nil {
				return false, fmt.Errorf("failed to delete failed backup job %s: %w", jobName, err)
			}

			// Return false to requeue and create new job on next reconcile
			return false, nil
		}

		// If job succeeded, we are done
		if existingJobStatus == "succeeded" {
			logger.Info("Pre-upgrade backup job completed successfully", "job", jobName)
			return true, nil
		}

		// If job is running, we wait
		logger.Info("Pre-upgrade backup job is still running", "job", jobName)
		return false, nil
	}

	// No job exists - create new job
	jobName = m.backupJobName(cluster)
	logger.Info("Creating pre-upgrade backup job", "job", jobName)

	if err := backup.EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster); err != nil {
		return false, operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err))
	}
	if err := backup.EnsureBackupRBAC(ctx, m.client, m.scheme, cluster); err != nil {
		return false, operatorerrors.WithReason(upgrade.ReasonPreUpgradeBackupFailed, fmt.Errorf("failed to ensure backup RBAC: %w", err))
	}

	verifiedExecutorDigest := ""
	executorImage := strings.TrimSpace(cluster.Spec.Backup.ExecutorImage)
	if executorImage != "" && cluster.Spec.OperatorImageVerification != nil && cluster.Spec.OperatorImageVerification.Enabled {
		verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
		defer cancel()

		digest, err := security.VerifyOperatorImageForCluster(verifyCtx, logger, m.operatorImageVerifier, cluster, executorImage)
		if err != nil {
			failurePolicy := cluster.Spec.OperatorImageVerification.FailurePolicy
			if failurePolicy == "" {
				failurePolicy = constants.ImageVerificationFailurePolicyBlock
			}
			if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
				return false, operatorerrors.WithReason(constants.ReasonPreUpgradeBackupImageVerificationFailed, fmt.Errorf("pre-upgrade backup executor image verification failed (policy=Block): %w", err))
			}

			logger.Error(err, "Pre-upgrade backup executor image verification failed but proceeding due to Warn policy", "image", executorImage)
		} else {
			verifiedExecutorDigest = digest
			logger.Info("Pre-upgrade backup executor image verified successfully", "digest", digest)
		}
	}

	// For rolling upgrades, target the base StatefulSet (no revision suffix).
	// TargetStatefulSetName defaults to cluster.Name when empty.
	job, err := backup.BuildJob(cluster, backup.JobOptions{
		JobName:                jobName,
		JobType:                backup.JobTypePreUpgrade,
		FilenamePrefix:         constants.BackupTypePreUpgrade,
		VerifiedExecutorDigest: verifiedExecutorDigest,
		// TargetStatefulSetName left empty - defaults to cluster.Name for rolling upgrades
	})

	if err != nil {
		return false, fmt.Errorf("failed to build backup job: %w", err)
	}

	// Set OwnerReference for garbage collection
	if m.scheme != nil {
		if err := controllerutil.SetControllerReference(cluster, job, m.scheme); err != nil {
			return false, fmt.Errorf("failed to set owner reference on backup job: %w", err)
		}
	}

	if err := m.client.Create(ctx, job); err != nil {
		return false, fmt.Errorf("failed to create backup job: %w", err)
	}

	logger.Info("Pre-upgrade backup job created", "job", jobName)
	// Return false to indicate snapshot is not yet complete (it was just created)
	return false, nil
}

// validateBackupConfig validates that backup configuration is present and valid.
func (m *Manager) validateBackupConfig(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return fmt.Errorf("backup configuration is required when preUpgradeSnapshot is enabled")
	}

	// Check if JWT Auth is configured (preferred method)
	hasJWTAuth := strings.TrimSpace(backupCfg.JWTAuthRole) != ""

	// Check if static token is configured (fallback method)
	hasTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""

	// At least one authentication method must be configured
	if !hasJWTAuth && !hasTokenSecret {
		return fmt.Errorf("backup authentication is required: either jwtAuthRole or tokenSecretRef must be set")
	}

	// If using token secret, verify it exists
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
				return fmt.Errorf("backup token Secret %s/%s not found", secretNamespace, backupCfg.TokenSecretRef.Name)
			}
			return fmt.Errorf("failed to get backup token Secret %s/%s: %w", secretNamespace, backupCfg.TokenSecretRef.Name, err)
		}
	}

	// ExecutorImage defaults to constants.DefaultBackupImage() when not specified

	return nil
}

// findExistingPreUpgradeBackupJob finds an existing pre-upgrade backup job for this cluster.
// Returns the job name and status ("running", "failed", "succeeded") if found, empty strings if not found.
func (m *Manager) findExistingPreUpgradeBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, string, error) {
	expectedJobName := m.backupJobName(cluster)
	job := &batchv1.Job{}

	if err := m.client.Get(ctx, types.NamespacedName{
		Name:      expectedJobName,
		Namespace: cluster.Namespace,
	}, job); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("failed to get backup job %s: %w", expectedJobName, err)
	}

	// Determine status
	if kube.JobSucceeded(job) {
		return job.Name, "succeeded", nil
	}
	if kube.JobFailed(job) {
		return job.Name, "failed", nil
	}
	return job.Name, "running", nil
}

// backupJobName generates a deterministic name for a pre-upgrade backup job.
// The name is based on cluster generation to ensure idempotency per upgrade operation.
func (m *Manager) backupJobName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return fmt.Sprintf("pre-upgrade-backup-%s-gen%d", cluster.Name, cluster.Generation)
}

// countFailedPreUpgradeBackupJobs counts how many failed pre-upgrade backup jobs exist for this cluster.
func (m *Manager) countFailedPreUpgradeBackupJobs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (int, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		constants.LabelAppInstance:       cluster.Name,
		constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:    cluster.Name,
		constants.LabelOpenBaoComponent:  backup.ComponentBackup,
		constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return 0, fmt.Errorf("failed to list backup jobs: %w", err)
	}

	count := 0
	for i := range jobList.Items {
		if kube.JobFailed(&jobList.Items[i]) {
			count++
		}
	}
	return count, nil
}

// deletePreUpgradeBackupJob deletes a specific pre-upgrade backup job.
func (m *Manager) deletePreUpgradeBackupJob(ctx context.Context, jobName, namespace string) error {
	job := &batchv1.Job{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: namespace,
	}, job); err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Use propagation policy to delete pods as well
	propagation := metav1.DeletePropagationBackground
	if err := m.client.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete job: %w", err)
	}

	return nil
}

// NOTE: buildBackupJob has been removed. Pre-upgrade backup jobs are now built using
// the shared backup.BuildJob function from internal/backup/job_builder.go.
