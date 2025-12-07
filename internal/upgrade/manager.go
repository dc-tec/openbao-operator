package upgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/logging"
	openbaoapi "github.com/openbao/operator/internal/openbao"
)

const (
	// tlsCASecretSuffix is the suffix for the TLS CA secret.
	tlsCASecretSuffix = "-tls-ca"
	// headlessServiceSuffix is appended to cluster name for the headless service.
	headlessServiceSuffix = ""
	// openBaoContainerPort is the API port.
	openBaoContainerPort = 8200
	// defaultK8sTokenPath is the default path for Kubernetes ServiceAccount tokens.
	defaultK8sTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

var (
	// ErrNoUpgradeToken indicates that no suitable upgrade token is configured.
	ErrNoUpgradeToken = errors.New("no upgrade token configured: either spec.upgrade.kubernetesAuthRole, spec.upgrade.tokenSecretRef, or (when preUpgradeSnapshot is enabled) spec.backup.kubernetesAuthRole or spec.backup.tokenSecretRef must be set")
)

// OpenBaoClientFactory creates OpenBao API clients for connecting to cluster pods.
// This is primarily used for testing to inject mock clients.
type OpenBaoClientFactory func(config openbaoapi.ClientConfig) (*openbaoapi.Client, error)

// Manager reconciles version and Raft-aware upgrade behavior for an OpenBaoCluster.
type Manager struct {
	client        client.Client
	scheme        *runtime.Scheme
	clientFactory OpenBaoClientFactory
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client:        c,
		scheme:        scheme,
		clientFactory: openbaoapi.NewClient,
	}
}

// NewManagerWithClientFactory constructs a Manager with a custom OpenBao client factory.
// This is primarily used for testing.
func NewManagerWithClientFactory(c client.Client, scheme *runtime.Scheme, factory OpenBaoClientFactory) *Manager {
	return &Manager{
		client:        c,
		scheme:        scheme,
		clientFactory: factory,
	}
}

// Reconcile ensures upgrades progress safely for the given OpenBaoCluster.
//
// The upgrade state machine follows these phases:
//  1. Detection: Check if upgrade is needed or if we're resuming an existing one
//  2. Pre-upgrade Validation: Validate version, check cluster health
//  3. Initialize Upgrade: Set up upgrade state, lock StatefulSet partition
//  4. Pod-by-Pod Update: Step down leader if needed, update each pod in reverse ordinal order
//  5. Finalization: Clear upgrade state, update current version
//
// The reconciler returns nil on success or when no action is needed.
// It returns an error to trigger requeue on transient failures.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger = logger.WithValues(
		"specVersion", cluster.Spec.Version,
		"statusVersion", cluster.Status.CurrentVersion,
	)

	metrics := NewMetrics(cluster.Namespace, cluster.Name)

	// Check if cluster is initialized; upgrades can only proceed after initialization
	if !cluster.Status.Initialized {
		logger.V(1).Info("Cluster not initialized; skipping upgrade reconciliation")
		return nil
	}

	// Phase 1: Detection - determine if upgrade is needed
	upgradeNeeded, resumeUpgrade := m.detectUpgradeState(logger, cluster)

	if !upgradeNeeded && !resumeUpgrade {
		// No upgrade needed, ensure metrics reflect idle state
		metrics.SetInProgress(false)
		metrics.SetStatus(UpgradeStatusNone)
		return nil
	}

	// Handle resume scenario where spec.version changed mid-upgrade
	if resumeUpgrade && cluster.Status.Upgrade != nil {
		if cluster.Spec.Version != cluster.Status.Upgrade.TargetVersion {
			logger.Info("Spec.Version changed during upgrade; clearing upgrade state and starting fresh",
				"previousTarget", cluster.Status.Upgrade.TargetVersion,
				"newTarget", cluster.Spec.Version)
			ClearUpgrade(&cluster.Status, ReasonVersionMismatch,
				fmt.Sprintf("Target version changed from %s to %s during upgrade",
					cluster.Status.Upgrade.TargetVersion, cluster.Spec.Version),
				cluster.Generation)
			// Continuing will re-evaluate and start fresh
		}
	}

	// Phase 2: Pre-upgrade Validation
	if err := m.validateUpgrade(ctx, logger, cluster); err != nil {
		return err
	}

	// Phase 3: Pre-upgrade Snapshot (if enabled)
	// Note: This happens after validation to ensure cluster is healthy before snapshot
	// If backup is in progress (snapshotComplete=false), this will return nil to requeue, preventing upgrade initialization
	if cluster.Status.Upgrade == nil {
		snapshotComplete, err := m.handlePreUpgradeSnapshot(ctx, logger, cluster)
		if err != nil {
			return err
		}
		if !snapshotComplete {
			logger.Info("Pre-upgrade snapshot in progress, waiting...")
			return nil
		}
	}

	// Phase 4: Initialize Upgrade (if not resuming)
	// Only reached if pre-upgrade snapshot is complete or not enabled
	if cluster.Status.Upgrade == nil {
		if err := m.initializeUpgrade(ctx, logger, cluster); err != nil {
			return err
		}
	}

	// Update metrics for in-progress upgrade
	metrics.SetInProgress(true)
	metrics.SetStatus(UpgradeStatusRunning)
	if cluster.Status.Upgrade != nil {
		metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
		metrics.SetTotalPods(int(cluster.Spec.Replicas))
	}

	// Phase 5: Pod-by-Pod Update
	completed, err := m.performPodByPodUpgrade(ctx, logger, cluster, metrics)
	if err != nil {
		SetUpgradeFailed(&cluster.Status, ReasonUpgradeFailed, err.Error(), cluster.Generation)
		metrics.SetStatus(UpgradeStatusFailed)
		if statusErr := m.client.Status().Update(ctx, cluster); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after upgrade failure")
		}
		return err
	}

	if !completed {
		// Upgrade is still in progress; save state and requeue
		if err := m.client.Status().Update(ctx, cluster); err != nil {
			return fmt.Errorf("failed to update upgrade progress: %w", err)
		}
		return nil
	}

	// Phase 6: Finalization
	if err := m.finalizeUpgrade(ctx, logger, cluster, metrics); err != nil {
		return err
	}

	return nil
}

// detectUpgradeState determines whether an upgrade is needed or if we're resuming one.
func (m *Manager) detectUpgradeState(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (upgradeNeeded bool, resumeUpgrade bool) {
	// If upgrade is already in progress, we're resuming
	if cluster.Status.Upgrade != nil {
		logger.Info("Resuming in-progress upgrade",
			"fromVersion", cluster.Status.Upgrade.FromVersion,
			"targetVersion", cluster.Status.Upgrade.TargetVersion,
			"currentPartition", cluster.Status.Upgrade.CurrentPartition)
		return false, true
	}

	// If current version is empty, this is the first reconcile after initialization
	// Set it to spec.version and don't trigger an upgrade
	if cluster.Status.CurrentVersion == "" {
		logger.Info("Setting initial CurrentVersion from spec",
			"version", cluster.Spec.Version)
		// This is handled in the main controller status update
		return false, false
	}

	// Check if spec version differs from current version
	if cluster.Spec.Version == cluster.Status.CurrentVersion {
		logger.V(1).Info("No upgrade needed; versions match")
		return false, false
	}

	// Version mismatch - upgrade is needed
	logger.Info("Upgrade detected",
		"from", cluster.Status.CurrentVersion,
		"to", cluster.Spec.Version)
	return true, false
}

// validateUpgrade performs pre-upgrade validation checks.
func (m *Manager) validateUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Validate target version format
	if err := ValidateVersion(cluster.Spec.Version); err != nil {
		SetInvalidVersion(&cluster.Status, cluster.Spec.Version, err, cluster.Generation)
		if statusErr := m.client.Status().Update(ctx, cluster); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after invalid version")
		}
		return fmt.Errorf("invalid target version: %w", err)
	}

	// Skip version comparison if this is resuming an upgrade or if no current version
	if cluster.Status.Upgrade == nil && cluster.Status.CurrentVersion != "" {
		// Check for downgrade
		if IsDowngrade(cluster.Status.CurrentVersion, cluster.Spec.Version) {
			logger.Info("Downgrade detected and blocked",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
			SetDowngradeBlocked(&cluster.Status, cluster.Status.CurrentVersion, cluster.Spec.Version, cluster.Generation)
			if statusErr := m.client.Status().Update(ctx, cluster); statusErr != nil {
				logger.Error(statusErr, "Failed to update status after downgrade block")
			}
			return fmt.Errorf("downgrade from %s to %s is not allowed",
				cluster.Status.CurrentVersion, cluster.Spec.Version)
		}

		// Log warning for minor version skips or major upgrades
		change, _ := CompareVersions(cluster.Status.CurrentVersion, cluster.Spec.Version)
		if change == VersionChangeMajor {
			logger.Info("Major version upgrade detected; proceed with caution",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
		}
		if IsSkipMinorUpgrade(cluster.Status.CurrentVersion, cluster.Spec.Version) {
			logger.Info("Minor version skip detected; some intermediate versions may be skipped",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
		}
	}

	// Verify cluster health
	if err := m.verifyClusterHealth(ctx, logger, cluster); err != nil {
		SetClusterNotReady(&cluster.Status, err.Error(), cluster.Generation)
		if statusErr := m.client.Status().Update(ctx, cluster); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after health check failure")
		}
		return err
	}

	return nil
}

// verifyClusterHealth checks that the cluster is in a state suitable for upgrades.
func (m *Manager) verifyClusterHealth(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Get the StatefulSet
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, stsName, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("StatefulSet not found; cluster may not be fully initialized")
		}
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Verify all replicas are ready
	if sts.Status.ReadyReplicas != cluster.Spec.Replicas {
		return fmt.Errorf("not all replicas are ready (%d/%d)",
			sts.Status.ReadyReplicas, cluster.Spec.Replicas)
	}

	// Get cluster pods and verify health
	podList, err := m.getClusterPods(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to list cluster pods: %w", err)
	}

	if len(podList) != int(cluster.Spec.Replicas) {
		return fmt.Errorf("unexpected number of pods (%d/%d)",
			len(podList), cluster.Spec.Replicas)
	}

	// Verify quorum - at least (replicas/2)+1 must be healthy
	healthyCount, leaderCount, err := m.checkPodHealth(ctx, logger, cluster, podList)
	if err != nil {
		return fmt.Errorf("failed to check pod health: %w", err)
	}

	quorumRequired := (cluster.Spec.Replicas / 2) + 1
	if healthyCount < int(quorumRequired) {
		return fmt.Errorf("cluster has lost quorum (%d/%d healthy, need %d)",
			healthyCount, cluster.Spec.Replicas, quorumRequired)
	}

	// Verify single leader
	if leaderCount == 0 {
		return fmt.Errorf("no leader found in cluster")
	}
	if leaderCount > 1 {
		return fmt.Errorf("multiple leaders detected (%d); possible split-brain", leaderCount)
	}

	logger.Info("Cluster health verified",
		"healthyPods", healthyCount,
		"totalPods", cluster.Spec.Replicas,
		"leaderCount", leaderCount)

	return nil
}

// checkPodHealth queries each pod's health status and returns counts.
func (m *Manager) checkPodHealth(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (healthyCount, leaderCount int, err error) {
	// Get CA cert for TLS connections
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get CA certificate: %w", err)
	}

	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podURL := m.getPodURL(cluster, pod.Name)
		apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
			BaseURL: podURL,
			CACert:  caCert,
		})
		if err != nil {
			logger.V(1).Info("Failed to create client for pod", "pod", pod.Name, "error", err)
			continue
		}

		healthy, err := apiClient.IsHealthy(ctx)
		if err != nil {
			logger.V(1).Info("Health check failed for pod", "pod", pod.Name, "error", err)
			continue
		}

		if healthy {
			healthyCount++
		}

		isLeader, err := apiClient.IsLeader(ctx)
		if err != nil {
			logger.V(1).Info("Leader check failed for pod", "pod", pod.Name, "error", err)
			continue
		}

		if isLeader {
			leaderCount++
			cluster.Status.ActiveLeader = pod.Name
		}
	}

	return healthyCount, leaderCount, nil
}

// handlePreUpgradeSnapshot checks if preUpgradeSnapshot is enabled and triggers a backup if needed.
// Returns true if the snapshot is complete (or disabled), false if it is in progress (created or running).
// Returns an error if backup fails, which will block the upgrade.
func (m *Manager) handlePreUpgradeSnapshot(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	// Check if pre-upgrade snapshot is enabled
	if cluster.Spec.Upgrade == nil || !cluster.Spec.Upgrade.PreUpgradeSnapshot {
		logger.V(1).Info("Pre-upgrade snapshot is not enabled")
		return true, nil
	}

	// Verify backup configuration is valid
	if err := m.validateBackupConfig(ctx, cluster); err != nil {
		SetPreUpgradeBackupFailed(&cluster.Status, err.Error(), cluster.Generation)
		if statusErr := m.client.Status().Update(ctx, cluster); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after pre-upgrade backup validation failure")
		}
		return false, fmt.Errorf("pre-upgrade backup configuration invalid: %w", err)
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

		// If job failed, return error immediately
		if existingJobStatus == "failed" {
			SetPreUpgradeBackupFailed(&cluster.Status, fmt.Sprintf("backup job %s failed", jobName), cluster.Generation)
			if statusErr := m.client.Status().Update(ctx, cluster); statusErr != nil {
				logger.Error(statusErr, "Failed to update status after pre-upgrade backup failure")
			}
			return false, fmt.Errorf("pre-upgrade backup job %s failed", jobName)
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

	job, err := m.buildBackupJob(cluster, jobName)
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

	// Check if Kubernetes Auth is configured (preferred method)
	hasKubernetesAuth := strings.TrimSpace(backupCfg.KubernetesAuthRole) != ""

	// Check if static token is configured (fallback method)
	hasTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""

	// At least one authentication method must be configured
	if !hasKubernetesAuth && !hasTokenSecret {
		return fmt.Errorf("backup authentication is required: either kubernetesAuthRole or tokenSecretRef must be set")
	}

	// If using token secret, verify it exists
	if hasTokenSecret {
		secretNamespace := cluster.Namespace
		if ns := strings.TrimSpace(backupCfg.TokenSecretRef.Namespace); ns != "" {
			secretNamespace = ns
		}

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

	// Verify executor image is configured
	if strings.TrimSpace(backupCfg.ExecutorImage) == "" {
		return fmt.Errorf("backup executor image is required (spec.backup.executorImage)")
	}

	return nil
}

// findExistingPreUpgradeBackupJob finds an existing pre-upgrade backup job for this cluster.
// Returns the job name and status ("running", "failed", "succeeded") if found, empty strings if not found.
func (m *Manager) findExistingPreUpgradeBackupJob(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, string, error) {
	jobList := &batchv1.JobList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "openbao-operator",
		"openbao.org/cluster":          cluster.Name,
		"openbao.org/component":        "backup",
		"openbao.org/backup-type":      "pre-upgrade",
	})

	if err := m.client.List(ctx, jobList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return "", "", fmt.Errorf("failed to list backup jobs: %w", err)
	}

	// Find the most recent job (prefer running, then failed, then succeeded)
	var runningJob, failedJob, succeededJob *batchv1.Job
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if job.Status.Succeeded > 0 {
			if succeededJob == nil {
				succeededJob = job
			}
		} else if job.Status.Failed > 0 {
			if failedJob == nil {
				failedJob = job
			}
		} else if job.Status.Succeeded == 0 && job.Status.Failed == 0 {
			// Job is still running or pending
			if runningJob == nil {
				runningJob = job
			}
		}
	}

	// Return running job first, then failed, then succeeded
	if runningJob != nil {
		return runningJob.Name, "running", nil
	}
	if failedJob != nil {
		return failedJob.Name, "failed", nil
	}
	if succeededJob != nil {
		return succeededJob.Name, "succeeded", nil
	}

	// No job found
	return "", "", nil
}

// backupJobName generates a unique name for a backup job.
func (m *Manager) backupJobName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	// Use a fixed prefix for pre-upgrade backups so we can detect existing jobs
	return fmt.Sprintf("pre-upgrade-backup-%s-%s", cluster.Name, time.Now().Format("20060102-150405"))
}

// buildBackupJob builds a Kubernetes Job for executing a backup.
// This mirrors the logic from internal/backup/job.go but is adapted for pre-upgrade use.
func (m *Manager) buildBackupJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName string) (*batchv1.Job, error) {
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return nil, fmt.Errorf("backup configuration is required")
	}

	// Build environment variables for the backup container
	env := []corev1.EnvVar{
		{Name: "CLUSTER_NAMESPACE", Value: cluster.Namespace},
		{Name: "CLUSTER_NAME", Value: cluster.Name},
		{Name: "CLUSTER_REPLICAS", Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: "BACKUP_ENDPOINT", Value: backupCfg.Target.Endpoint},
		{Name: "BACKUP_BUCKET", Value: backupCfg.Target.Bucket},
		{Name: "BACKUP_PATH_PREFIX", Value: backupCfg.Target.PathPrefix},
		{Name: "BACKUP_FILENAME_PREFIX", Value: "pre-upgrade"}, // Prefix filenames with "pre-upgrade-"
		{Name: "BACKUP_USE_PATH_STYLE", Value: fmt.Sprintf("%t", backupCfg.Target.UsePathStyle)},
	}

	// Add credentials secret reference if provided
	if backupCfg.Target.CredentialsSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_CREDENTIALS_SECRET_NAME",
			Value: backupCfg.Target.CredentialsSecretRef.Name,
		})
		if backupCfg.Target.CredentialsSecretRef.Namespace != "" {
			env = append(env, corev1.EnvVar{
				Name:  "BACKUP_CREDENTIALS_SECRET_NAMESPACE",
				Value: backupCfg.Target.CredentialsSecretRef.Namespace,
			})
		}
	}

	// Add Kubernetes Auth configuration (preferred method)
	if backupCfg.KubernetesAuthRole != "" {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_KUBERNETES_AUTH_ROLE",
			Value: backupCfg.KubernetesAuthRole,
		})
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_AUTH_METHOD",
			Value: "kubernetes",
		})
	}

	// Add token secret reference if provided (fallback for token-based auth)
	if backupCfg.TokenSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  "BACKUP_TOKEN_SECRET_NAME",
			Value: backupCfg.TokenSecretRef.Name,
		})
		if backupCfg.TokenSecretRef.Namespace != "" {
			env = append(env, corev1.EnvVar{
				Name:  "BACKUP_TOKEN_SECRET_NAMESPACE",
				Value: backupCfg.TokenSecretRef.Namespace,
			})
		}
		// Only set auth method to token if Kubernetes Auth is not configured
		if backupCfg.KubernetesAuthRole == "" {
			env = append(env, corev1.EnvVar{
				Name:  "BACKUP_AUTH_METHOD",
				Value: "token",
			})
		}
	}

	backoffLimit := int32(0)               // Don't retry failed backups automatically
	ttlSecondsAfterFinished := int32(3600) // 1 hour TTL

	image := strings.TrimSpace(backupCfg.ExecutorImage)
	if image == "" {
		return nil, fmt.Errorf("backup executor image is required")
	}

	// Build volumes and volume mounts
	volumes := []corev1.Volume{
		{
			Name: "tls-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: fmt.Sprintf("%s-tls-ca", cluster.Name),
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tls-ca",
			MountPath: "/etc/bao/tls",
			ReadOnly:  true,
		},
	}

	// Add credentials secret volume if provided
	if backupCfg.Target.CredentialsSecretRef != nil {
		credentialsFileMode := int32(0400)
		volumes = append(volumes, corev1.Volume{
			Name: "backup-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  backupCfg.Target.CredentialsSecretRef.Name,
					DefaultMode: &credentialsFileMode,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "backup-credentials",
			MountPath: "/etc/bao/backup/credentials",
			ReadOnly:  true,
		})
	}

	// Add token secret volume if provided
	if backupCfg.TokenSecretRef != nil {
		tokenFileMode := int32(0400)
		volumes = append(volumes, corev1.Volume{
			Name: "backup-token",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  backupCfg.TokenSecretRef.Name,
					DefaultMode: &tokenFileMode,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "backup-token",
			MountPath: "/etc/bao/backup/token",
			ReadOnly:  true,
		})
	}

	// Get backup ServiceAccount name
	backupServiceAccountName := cluster.Name + "-backup-serviceaccount"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   cluster.Name,
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          cluster.Name,
				"openbao.org/component":        "backup",
				"openbao.org/backup-type":      "pre-upgrade",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":       "openbao",
						"app.kubernetes.io/instance":   cluster.Name,
						"app.kubernetes.io/managed-by": "openbao-operator",
						"openbao.org/cluster":          cluster.Name,
						"openbao.org/component":        "backup",
						"openbao.org/backup-type":      "pre-upgrade",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: backupServiceAccountName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(1000)),
						RunAsGroup:   ptr.To(int64(1000)),
						FSGroup:      ptr.To(int64(1000)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:         "backup",
							Image:        image,
							Env:          env,
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	return job, nil
}

// initializeUpgrade sets up the upgrade state and locks the StatefulSet partition.
func (m *Manager) initializeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	fromVersion := cluster.Status.CurrentVersion
	toVersion := cluster.Spec.Version

	logger.Info("Initializing upgrade",
		"from", fromVersion,
		"to", toVersion,
		"replicas", cluster.Spec.Replicas)

	// Set upgrade state
	SetUpgradeStarted(&cluster.Status, fromVersion, toVersion, cluster.Spec.Replicas, cluster.Generation)

	// Lock StatefulSet by setting partition to replicas (prevents all updates)
	if err := m.setStatefulSetPartition(ctx, cluster, cluster.Spec.Replicas); err != nil {
		return fmt.Errorf("failed to lock StatefulSet partition: %w", err)
	}

	// Update status
	if err := m.client.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status after initializing upgrade: %w", err)
	}

	logger.Info("Upgrade initialized; StatefulSet partition locked",
		"partition", cluster.Spec.Replicas)

	return nil
}

// performPodByPodUpgrade executes the rolling update, one pod at a time.
// Returns true when all pods have been upgraded.
func (m *Manager) performPodByPodUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *Metrics) (bool, error) {
	if cluster.Status.Upgrade == nil {
		return false, fmt.Errorf("upgrade state is nil")
	}

	currentPartition := cluster.Status.Upgrade.CurrentPartition

	// If partition is 0, all pods have been updated
	if currentPartition == 0 {
		logger.Info("All pods have been updated")
		return true, nil
	}

	// The next pod to update is at ordinal (partition - 1)
	targetOrdinal := currentPartition - 1
	podName := fmt.Sprintf("%s-%d", cluster.Name, targetOrdinal)

	logger.Info("Processing pod for upgrade",
		"pod", podName,
		"ordinal", targetOrdinal,
		"partition", currentPartition)

	podStartTime := time.Now()

	// Check if this pod is the leader
	isLeader, err := m.isPodLeader(ctx, cluster, podName)
	if err != nil {
		return false, fmt.Errorf("failed to check if pod %s is leader: %w", podName, err)
	}

	if isLeader {
		logger.Info("Pod is the leader; initiating step-down", "pod", podName)
		if err := m.stepDownLeader(ctx, logger, cluster, podName, metrics); err != nil {
			return false, err
		}
	}

	// Decrement partition to allow this pod to update
	newPartition := currentPartition - 1
	if err := m.setStatefulSetPartition(ctx, cluster, newPartition); err != nil {
		return false, fmt.Errorf("failed to update partition: %w", err)
	}

	// Wait for the pod to become ready with new version
	if err := m.waitForPodReady(ctx, logger, cluster, podName); err != nil {
		return false, err
	}

	// Wait for OpenBao to be healthy on this pod
	if err := m.waitForPodHealthy(ctx, logger, cluster, podName); err != nil {
		return false, err
	}

	// Update progress
	SetUpgradeProgress(&cluster.Status, newPartition, targetOrdinal, cluster.Spec.Replicas, cluster.Generation)

	// Record pod upgrade duration
	podDuration := time.Since(podStartTime).Seconds()
	metrics.RecordPodDuration(podDuration, podName)
	metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))

	logger.Info("Pod upgrade completed",
		"pod", podName,
		"duration", podDuration,
		"remainingPartition", newPartition)

	// Check if there are more pods to update
	if newPartition > 0 {
		return false, nil
	}

	return true, nil
}

// isPodLeader checks if a specific pod is the Raft leader.
func (m *Manager) isPodLeader(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to get CA certificate: %w", err)
	}

	podURL := m.getPodURL(cluster, podName)
	apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
		BaseURL: podURL,
		CACert:  caCert,
	})
	if err != nil {
		return false, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	return apiClient.IsLeader(ctx)
}

// stepDownLeader performs a leader step-down and waits for leadership transfer.
func (m *Manager) stepDownLeader(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string, metrics *Metrics) error {
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to get CA certificate: %w", err)
	}

	token, err := m.getOperatorToken(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to get operator token: %w", err)
	}

	podURL := m.getPodURL(cluster, podName)
	apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
		BaseURL: podURL,
		CACert:  caCert,
		Token:   token,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	// Record step-down attempt
	metrics.IncrementStepDownTotal()

	// Audit log: Leader step-down operation
	logging.LogAuditEvent(logger, "StepDown", map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"pod":               podName,
		"target_version":    cluster.Status.Upgrade.TargetVersion,
		"from_version":      cluster.Status.Upgrade.FromVersion,
	})

	// Execute step-down
	if err := apiClient.StepDown(ctx); err != nil {
		metrics.IncrementStepDownFailures()
		logging.LogAuditEvent(logger, "StepDownFailed", map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
			"error":             err.Error(),
		})
		return fmt.Errorf("step-down failed: %w", err)
	}

	// Wait for leadership to transfer
	stepDownCtx, cancel := context.WithTimeout(ctx, DefaultStepDownTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultLeaderCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stepDownCtx.Done():
			metrics.IncrementStepDownFailures()
			SetUpgradeFailed(&cluster.Status, ReasonStepDownTimeout,
				fmt.Sprintf(MessageStepDownTimeout, DefaultStepDownTimeout),
				cluster.Generation)
			return fmt.Errorf("step-down timeout: leadership did not transfer within %v", DefaultStepDownTimeout)
		case <-ticker.C:
			isStillLeader, err := apiClient.IsLeader(ctx)
			if err != nil {
				logger.V(1).Info("Error checking leader status after step-down", "error", err)
				continue
			}
			if !isStillLeader {
				logger.Info("Leadership transferred successfully", "previousLeader", podName)
				SetStepDownPerformed(&cluster.Status)
				logging.LogAuditEvent(logger, "StepDownCompleted", map[string]string{
					"cluster_namespace": cluster.Namespace,
					"cluster_name":      cluster.Name,
					"pod":               podName,
				})
				return nil
			}
			logger.V(1).Info("Waiting for leadership transfer", "pod", podName)
		}
	}
}

// setStatefulSetPartition updates the StatefulSet's partition value.
func (m *Manager) setStatefulSetPartition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, partition int32) error {
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	if err := m.client.Get(ctx, stsName, sts); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Ensure RollingUpdate strategy exists
	if sts.Spec.UpdateStrategy.RollingUpdate == nil {
		sts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
		sts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
	}

	sts.Spec.UpdateStrategy.RollingUpdate.Partition = &partition

	if err := m.client.Update(ctx, sts); err != nil {
		return fmt.Errorf("failed to update StatefulSet partition: %w", err)
	}

	return nil
}

// waitForPodReady waits for a pod to become Ready after being updated.
func (m *Manager) waitForPodReady(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) error {
	readyCtx, cancel := context.WithTimeout(ctx, DefaultPodReadyTimeout)
	defer cancel()

	ticker := time.NewTicker(DefaultPodReadyCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-readyCtx.Done():
			SetUpgradeFailed(&cluster.Status, ReasonPodNotReady,
				fmt.Sprintf(MessagePodNotReady, podName, DefaultPodReadyTimeout),
				cluster.Generation)
			return fmt.Errorf("pod %s did not become ready within %v", podName, DefaultPodReadyTimeout)
		case <-ticker.C:
			pod := &corev1.Pod{}
			if err := m.client.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      podName,
			}, pod); err != nil {
				if apierrors.IsNotFound(err) {
					logger.V(1).Info("Pod not found yet; waiting", "pod", podName)
					continue
				}
				return fmt.Errorf("failed to get pod %s: %w", podName, err)
			}

			if isPodReady(pod) {
				logger.Info("Pod is ready", "pod", podName)
				return nil
			}

			logger.V(1).Info("Waiting for pod to become ready", "pod", podName, "phase", pod.Status.Phase)
		}
	}
}

// waitForPodHealthy waits for OpenBao to become healthy on a pod.
func (m *Manager) waitForPodHealthy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) error {
	healthCtx, cancel := context.WithTimeout(ctx, DefaultHealthCheckTimeout)
	defer cancel()

	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return fmt.Errorf("failed to get CA certificate: %w", err)
	}

	podURL := m.getPodURL(cluster, podName)
	apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
		BaseURL: podURL,
		CACert:  caCert,
	})
	if err != nil {
		return fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	ticker := time.NewTicker(DefaultHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-healthCtx.Done():
			SetUpgradeFailed(&cluster.Status, ReasonHealthCheckFailed,
				fmt.Sprintf(MessageHealthCheckFailed, podName, "timeout"),
				cluster.Generation)
			return fmt.Errorf("OpenBao health check timeout for pod %s", podName)
		case <-ticker.C:
			healthy, err := apiClient.IsHealthy(ctx)
			if err != nil {
				logger.V(1).Info("Health check error; retrying", "pod", podName, "error", err)
				continue
			}
			if healthy {
				logger.Info("OpenBao is healthy on pod", "pod", podName)
				return nil
			}
			logger.V(1).Info("Waiting for OpenBao to become healthy", "pod", podName)
		}
	}
}

// finalizeUpgrade completes the upgrade process.
func (m *Manager) finalizeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *Metrics) error {
	var upgradeDuration float64
	var fromVersion string

	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		upgradeDuration = time.Since(cluster.Status.Upgrade.StartedAt.Time).Seconds()
		fromVersion = cluster.Status.Upgrade.FromVersion
	}

	// Mark upgrade complete
	SetUpgradeComplete(&cluster.Status, cluster.Spec.Version, cluster.Generation)

	// Update status
	if err := m.client.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status after completing upgrade: %w", err)
	}

	// Record metrics
	if upgradeDuration > 0 {
		metrics.RecordDuration(upgradeDuration, fromVersion, cluster.Spec.Version)
	}
	metrics.SetInProgress(false)
	metrics.SetStatus(UpgradeStatusSuccess)
	metrics.SetPodsCompleted(0)
	metrics.SetTotalPods(0)

	logger.Info("Upgrade completed successfully",
		"version", cluster.Spec.Version,
		"duration", upgradeDuration)

	return nil
}

// getClusterPods returns all pods belonging to the cluster.
// It filters out backup job pods and other non-StatefulSet pods.
func (m *Manager) getClusterPods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/name":       "openbao",
		"app.kubernetes.io/managed-by": "openbao-operator",
	})

	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, err
	}

	// Filter out backup job pods and other non-StatefulSet pods
	// StatefulSet pods have a name pattern: <cluster-name>-<ordinal>
	// Backup job pods have labels like "openbao.org/component": "backup"
	filteredPods := make([]corev1.Pod, 0, len(podList.Items))
	statefulSetPrefix := cluster.Name + "-"
	for _, pod := range podList.Items {
		// Skip backup job pods (they have the backup component label)
		if pod.Labels["openbao.org/component"] == "backup" {
			continue
		}
		// Only include pods that match the StatefulSet naming pattern
		// StatefulSet pods are named like: <cluster-name>-<ordinal>
		if !strings.HasPrefix(pod.Name, statefulSetPrefix) {
			continue
		}
		// Extract the suffix after the cluster name
		suffix := pod.Name[len(statefulSetPrefix):]
		// Verify the suffix is a valid ordinal (numeric)
		if ordinal, err := strconv.Atoi(suffix); err == nil && ordinal >= 0 {
			filteredPods = append(filteredPods, pod)
		}
	}

	// Sort by ordinal (descending) for consistent processing order
	sort.Slice(filteredPods, func(i, j int) bool {
		ordinalI := extractOrdinal(filteredPods[i].Name)
		ordinalJ := extractOrdinal(filteredPods[j].Name)
		return ordinalI > ordinalJ
	})

	return filteredPods, nil
}

// getClusterCACert retrieves the cluster's CA certificate for TLS connections.
func (m *Manager) getClusterCACert(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	secretName := cluster.Name + tlsCASecretSuffix
	secret := &corev1.Secret{}

	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secretName,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get CA secret %s: %w", secretName, err)
	}

	caCert, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, fmt.Errorf("CA certificate not found in secret %s", secretName)
	}

	return caCert, nil
}

// getOperatorToken retrieves the authentication token for OpenBao API calls.
// It supports Kubernetes Auth (preferred) and static token authentication.
// When spec.upgrade.preUpgradeSnapshot is true, it falls back to backup authentication
// if upgrade authentication is not explicitly configured.
func (m *Manager) getOperatorToken(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	// Determine which authentication configuration to use
	upgradeCfg := cluster.Spec.Upgrade
	backupCfg := cluster.Spec.Backup
	useBackupAuth := false

	// Check if upgrade auth is explicitly configured
	hasUpgradeKubernetesAuth := upgradeCfg != nil && strings.TrimSpace(upgradeCfg.KubernetesAuthRole) != ""
	hasUpgradeTokenSecret := upgradeCfg != nil && upgradeCfg.TokenSecretRef != nil && strings.TrimSpace(upgradeCfg.TokenSecretRef.Name) != ""

	// If upgrade auth is not configured and preUpgradeSnapshot is enabled, use backup auth
	if !hasUpgradeKubernetesAuth && !hasUpgradeTokenSecret {
		if upgradeCfg != nil && upgradeCfg.PreUpgradeSnapshot {
			useBackupAuth = true
			hasBackupKubernetesAuth := strings.TrimSpace(backupCfg.KubernetesAuthRole) != ""
			hasBackupTokenSecret := backupCfg.TokenSecretRef != nil && strings.TrimSpace(backupCfg.TokenSecretRef.Name) != ""

			if !hasBackupKubernetesAuth && !hasBackupTokenSecret {
				return "", fmt.Errorf("preUpgradeSnapshot is enabled but no backup authentication configured: %w", ErrNoUpgradeToken)
			}
		} else {
			return "", ErrNoUpgradeToken
		}
	}

	// Determine which Kubernetes Auth role to use
	var kubernetesAuthRole string
	if hasUpgradeKubernetesAuth {
		kubernetesAuthRole = strings.TrimSpace(upgradeCfg.KubernetesAuthRole)
	} else if useBackupAuth && strings.TrimSpace(backupCfg.KubernetesAuthRole) != "" {
		kubernetesAuthRole = strings.TrimSpace(backupCfg.KubernetesAuthRole)
	}

	// If Kubernetes Auth is configured, use it
	if kubernetesAuthRole != "" {
		return m.authenticateWithKubernetesAuth(ctx, cluster, kubernetesAuthRole)
	}

	// Otherwise, use static token
	var tokenSecretRef *corev1.SecretReference
	if hasUpgradeTokenSecret {
		tokenSecretRef = upgradeCfg.TokenSecretRef
	} else if useBackupAuth && backupCfg.TokenSecretRef != nil {
		tokenSecretRef = backupCfg.TokenSecretRef
	}

	if tokenSecretRef == nil {
		return "", ErrNoUpgradeToken
	}

	return m.getTokenFromSecret(ctx, cluster, tokenSecretRef)
}

// authenticateWithKubernetesAuth authenticates to OpenBao using Kubernetes Auth.
func (m *Manager) authenticateWithKubernetesAuth(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, role string) (string, error) {
	// Read ServiceAccount token
	tokenPath := defaultK8sTokenPath
	k8sTokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read Kubernetes ServiceAccount token from %s: %w", tokenPath, err)
	}
	k8sToken := strings.TrimSpace(string(k8sTokenBytes))
	if k8sToken == "" {
		return "", fmt.Errorf("Kubernetes ServiceAccount token is empty")
	}

	// Get CA certificate for TLS
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("failed to get CA certificate: %w", err)
	}

	// Find pods to authenticate against
	podList, err := m.getClusterPods(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster pods: %w", err)
	}

	if len(podList) == 0 {
		return "", fmt.Errorf("no pods found in cluster")
	}

	// Try to authenticate against each pod until one succeeds
	var lastErr error
	for _, pod := range podList {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podURL := m.getPodURL(cluster, pod.Name)

		// Create OpenBao client without token for authentication
		apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
			BaseURL: podURL,
			CACert:  caCert,
		})
		if err != nil {
			lastErr = fmt.Errorf("failed to create OpenBao client for pod %s: %w", pod.Name, err)
			continue
		}

		// Authenticate using Kubernetes Auth
		token, err := apiClient.KubernetesAuthLogin(ctx, role, k8sToken)
		if err != nil {
			lastErr = fmt.Errorf("failed to authenticate using Kubernetes Auth against pod %s: %w", pod.Name, err)
			continue
		}

		return token, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("failed to authenticate against any pod: %w", lastErr)
	}

	return "", fmt.Errorf("no running pods available for authentication")
}

// getTokenFromSecret retrieves a token from a Kubernetes Secret.
func (m *Manager) getTokenFromSecret(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, secretRef *corev1.SecretReference) (string, error) {
	secretNamespace := cluster.Namespace
	if ns := strings.TrimSpace(secretRef.Namespace); ns != "" {
		secretNamespace = ns
	}

	secretName := types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretRef.Name,
	}

	secret := &corev1.Secret{}
	if err := m.client.Get(ctx, secretName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("upgrade token Secret %s/%s not found: %w", secretNamespace, secretRef.Name, ErrNoUpgradeToken)
		}
		return "", fmt.Errorf("failed to get upgrade token Secret %s/%s: %w", secretNamespace, secretRef.Name, err)
	}

	// Extract token from secret (default key is "token")
	tokenKey := "token"
	if secret.Data == nil {
		return "", fmt.Errorf("upgrade token Secret %s/%s has no data", secretNamespace, secretRef.Name)
	}

	token, ok := secret.Data[tokenKey]
	if !ok || len(token) == 0 {
		return "", fmt.Errorf("upgrade token Secret %s/%s missing %q key", secretNamespace, secretRef.Name, tokenKey)
	}

	return strings.TrimSpace(string(token)), nil
}

// getPodURL returns the URL for connecting to a specific pod.
func (m *Manager) getPodURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	// Use the pod's direct DNS name for the headless service
	// Format: <pod-name>.<service-name>.<namespace>.svc:<port>
	serviceName := cluster.Name + headlessServiceSuffix
	return fmt.Sprintf("https://%s.%s.%s.svc:%d",
		podName, serviceName, cluster.Namespace, openBaoContainerPort)
}

// isPodReady checks if a pod has the Ready condition set to True.
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// extractOrdinal extracts the ordinal number from a StatefulSet pod name.
// For example, "cluster-2" returns 2.
func extractOrdinal(podName string) int {
	parts := strings.Split(podName, "-")
	if len(parts) < 2 {
		return 0
	}
	ordinal, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return ordinal
}
