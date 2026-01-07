package upgrade

import (
	"context"
	"errors"
	"fmt"
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

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/kube"
	"github.com/dc-tec/openbao-operator/internal/logging"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/security"
)

const (
	// headlessServiceSuffix is appended to cluster name for the headless service.
	headlessServiceSuffix = ""
)

var (
	// ErrNoUpgradeToken indicates that no suitable upgrade token is configured.
	ErrNoUpgradeToken = errors.New("no upgrade token configured: either spec.upgrade.jwtAuthRole or spec.upgrade.tokenSecretRef must be set")
)

// OpenBaoClientFactory creates OpenBao API clients for connecting to cluster pods.
// This is primarily used for testing to inject mock clients.
type OpenBaoClientFactory func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error)

// Manager reconciles version and Raft-aware upgrade behavior for an OpenBaoCluster.
type Manager struct {
	client        client.Client
	scheme        *runtime.Scheme
	clientFactory OpenBaoClientFactory
}

// NewManager constructs a Manager that uses the provided Kubernetes client and scheme.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: scheme,
		clientFactory: func(config openbaoapi.ClientConfig) (openbaoapi.ClusterActions, error) {
			return openbaoapi.NewClient(config)
		},
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
// Returns (shouldRequeue, error) where shouldRequeue indicates if reconciliation should be requeued immediately.
//
//nolint:gocyclo // Upgrade reconciliation is an orchestrator with guard clauses and phase transitions.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {

	logger = logger.WithValues(
		"specVersion", cluster.Spec.Version,
		"statusVersion", cluster.Status.CurrentVersion,
	)

	metrics := NewMetrics(cluster.Namespace, cluster.Name)

	// Check if cluster is initialized; upgrades can only proceed after initialization
	if !cluster.Status.Initialized {
		logger.V(1).Info("Cluster not initialized; skipping upgrade reconciliation")
		return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
	}

	if cluster.Status.BreakGlass != nil && cluster.Status.BreakGlass.Active && cluster.Spec.BreakGlassAck != cluster.Status.BreakGlass.Nonce {
		logger.Info("Cluster is in break glass mode; halting upgrade reconciliation",
			"breakGlassReason", cluster.Status.BreakGlass.Reason,
			"breakGlassNonce", cluster.Status.BreakGlass.Nonce)
		return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
	}

	// Phase 1: Detection - determine if upgrade is needed
	upgradeNeeded, resumeUpgrade := m.detectUpgradeState(logger, cluster)

	if !upgradeNeeded && !resumeUpgrade {
		if cluster.Status.OperationLock != nil &&
			cluster.Status.OperationLock.Operation == openbaov1alpha1.ClusterOperationUpgrade &&
			cluster.Status.OperationLock.Holder == constants.ControllerNameOpenBaoCluster+"/upgrade" {
			if err := operationlock.Release(ctx, m.client, cluster, constants.ControllerNameOpenBaoCluster+"/upgrade", openbaov1alpha1.ClusterOperationUpgrade); err != nil && !errors.Is(err, operationlock.ErrLockHeld) {
				logger.Error(err, "Failed to release stale upgrade operation lock")
			}
		}

		// No upgrade needed, ensure metrics reflect idle state
		metrics.SetInProgress(false)
		metrics.SetStatus(UpgradeStatusNone)
		return recon.Result{}, nil
	}

	lockMessage := fmt.Sprintf("upgrade to %s", cluster.Spec.Version)
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.TargetVersion != "" {
		lockMessage = fmt.Sprintf("upgrade to %s (in progress)", cluster.Status.Upgrade.TargetVersion)
	}
	if err := operationlock.Acquire(ctx, m.client, cluster, operationlock.AcquireOptions{
		Holder:    constants.ControllerNameOpenBaoCluster + "/upgrade",
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   lockMessage,
	}); err != nil {
		if errors.Is(err, operationlock.ErrLockHeld) {
			if cluster.Status.Upgrade != nil {
				SetUpgradeFailed(&cluster.Status, ReasonUpgradeFailed, "upgrade halted due to concurrent operation lock", cluster.Generation)
				metrics.SetStatus(UpgradeStatusFailed)
				return recon.Result{}, fmt.Errorf("upgrade in progress but operation lock is held by another operation: %w", err)
			}
			logger.Info("Upgrade blocked by operation lock", "error", err.Error())
			return recon.Result{RequeueAfter: constants.RequeueStandard}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to acquire upgrade operation lock: %w", err)
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

	// Ensure upgrade ServiceAccount exists (for JWT Auth)
	if err := m.ensureUpgradeServiceAccount(ctx, logger, cluster); err != nil {
		return recon.Result{}, fmt.Errorf("failed to ensure upgrade ServiceAccount: %w", err)
	}

	// Phase 2: Pre-upgrade Validation
	if err := m.validateUpgrade(ctx, logger, cluster); err != nil {
		return recon.Result{}, err
	}

	// Phase 3: Pre-upgrade Snapshot (if enabled)
	// Note: This happens after validation to ensure cluster is healthy before snapshot
	// If backup is in progress (snapshotComplete=false), this will return nil to requeue, preventing upgrade initialization
	// We check this even if Status.Upgrade is set to handle edge cases where upgrade was initialized
	// but snapshot is still running (shouldn't happen, but we check defensively)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.PreUpgradeSnapshot {
		// Check if pre-upgrade snapshot is required and complete
		snapshotComplete, err := m.handlePreUpgradeSnapshot(ctx, logger, cluster)
		if err != nil {
			return recon.Result{}, err
		}
		if !snapshotComplete {
			logger.Info("Pre-upgrade snapshot in progress, waiting...")
			// If upgrade was already initialized, we should not proceed with pod updates
			// Return early to wait for snapshot completion
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}
	}

	// Phase 4: Initialize Upgrade (if not resuming)
	// Only reached if pre-upgrade snapshot is complete or not enabled
	if cluster.Status.Upgrade == nil {
		if err := m.initializeUpgrade(ctx, logger, cluster); err != nil {
			return recon.Result{}, err
		}
	}

	// Update metrics for in-progress upgrade
	metrics.SetInProgress(true)
	metrics.SetStatus(UpgradeStatusRunning)
	if cluster.Status.Upgrade != nil {
		metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
		metrics.SetTotalPods(int(cluster.Spec.Replicas))
		metrics.SetPartition(cluster.Status.Upgrade.CurrentPartition)
	}

	// Phase 5: Pod-by-Pod Update
	completed, err := m.performPodByPodUpgrade(ctx, logger, cluster, metrics)
	if err != nil {
		SetUpgradeFailed(&cluster.Status, ReasonUpgradeFailed, err.Error(), cluster.Generation)
		metrics.SetStatus(UpgradeStatusFailed)

		// Refresh cluster to get latest version before patching to avoid resource version conflicts
		freshCluster := &openbaov1alpha1.OpenBaoCluster{}
		if refreshErr := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		}, freshCluster); refreshErr != nil {
			logger.Error(refreshErr, "Failed to refresh cluster before status patch")
			return recon.Result{}, fmt.Errorf("failed to refresh cluster: %w", refreshErr)
		}
		// Use fresh cluster as base and apply our status changes
		freshBase := freshCluster.DeepCopy()
		freshCluster.Status.Upgrade = cluster.Status.Upgrade
		if statusErr := m.client.Status().Patch(ctx, freshCluster, client.MergeFrom(freshBase)); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after upgrade failure")
		}
		return recon.Result{}, err
	}

	if !completed {
		// Upgrade is still in progress; save state and requeue
		// Refresh cluster to get latest version before patching to avoid resource version conflicts
		freshCluster := &openbaov1alpha1.OpenBaoCluster{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		}, freshCluster); err != nil {
			return recon.Result{}, fmt.Errorf("failed to refresh cluster: %w", err)
		}
		// Use fresh cluster as base and apply our status changes
		freshBase := freshCluster.DeepCopy()
		freshCluster.Status.Upgrade = cluster.Status.Upgrade
		if err := m.client.Status().Patch(ctx, freshCluster, client.MergeFrom(freshBase)); err != nil {
			return recon.Result{}, fmt.Errorf("failed to update upgrade progress: %w", err)
		}
		return recon.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	// Phase 6: Finalization
	if err := m.finalizeUpgrade(ctx, logger, cluster, metrics); err != nil {
		return recon.Result{}, err
	}

	return recon.Result{}, nil
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
		return fmt.Errorf("invalid target version: %w", err)
	}

	// Skip version comparison if this is resuming an upgrade or if no current version
	if cluster.Status.Upgrade == nil && cluster.Status.CurrentVersion != "" {
		// Check for downgrade
		if IsDowngrade(cluster.Status.CurrentVersion, cluster.Spec.Version) {
			logger.Info("Downgrade detected and blocked",
				"from", cluster.Status.CurrentVersion,
				"to", cluster.Spec.Version)
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
			ClusterKey: fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name),
			BaseURL:    podURL,
			CACert:     caCert,
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

		isLeader, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil {
			logger.V(1).Info("Invalid OpenBao leader label value", "pod", pod.Name, "error", err)
			continue
		}

		if !present {
			isLeader, err = apiClient.IsLeader(ctx)
			if err != nil {
				logger.V(1).Info("Leader check failed for pod", "pod", pod.Name, "error", err)
				continue
			}
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
		return false, operatorerrors.WithReason(ReasonPreUpgradeBackupFailed, fmt.Errorf("pre-upgrade backup configuration invalid: %w", err))
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

	if err := backup.EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster); err != nil {
		return false, operatorerrors.WithReason(ReasonPreUpgradeBackupFailed, fmt.Errorf("failed to ensure backup ServiceAccount: %w", err))
	}
	if err := backup.EnsureBackupRBAC(ctx, m.client, m.scheme, cluster); err != nil {
		return false, operatorerrors.WithReason(ReasonPreUpgradeBackupFailed, fmt.Errorf("failed to ensure backup RBAC: %w", err))
	}

	verifiedExecutorDigest := ""
	executorImage := strings.TrimSpace(cluster.Spec.Backup.ExecutorImage)
	if executorImage != "" && cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled {
		verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
		defer cancel()

		digest, err := security.VerifyImageForCluster(verifyCtx, logger, m.client, cluster, executorImage)
		if err != nil {
			failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
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

	job, err := m.buildBackupJob(cluster, jobName, verifiedExecutorDigest)
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
		return "", "", fmt.Errorf("failed to list backup jobs: %w", err)
	}

	// Find the most recent job (prefer running, then failed, then succeeded)
	var runningJob, failedJob, succeededJob *batchv1.Job
	for i := range jobList.Items {
		job := &jobList.Items[i]
		if kube.JobSucceeded(job) {
			if succeededJob == nil {
				succeededJob = job
			}
		} else if kube.JobFailed(job) {
			if failedJob == nil {
				failedJob = job
			}
		} else {
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
func (m *Manager) buildBackupJob(cluster *openbaov1alpha1.OpenBaoCluster, jobName string, verifiedExecutorDigest string) (*batchv1.Job, error) {
	backupCfg := cluster.Spec.Backup
	if backupCfg == nil {
		return nil, fmt.Errorf("backup configuration is required")
	}

	// Build environment variables for the backup container
	env := []corev1.EnvVar{
		{Name: constants.EnvClusterNamespace, Value: cluster.Namespace},
		{Name: constants.EnvClusterName, Value: cluster.Name},
		{Name: constants.EnvClusterReplicas, Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: constants.EnvBackupEndpoint, Value: backupCfg.Target.Endpoint},
		{Name: constants.EnvBackupBucket, Value: backupCfg.Target.Bucket},
		{Name: constants.EnvBackupPathPrefix, Value: backupCfg.Target.PathPrefix},
		{Name: constants.EnvBackupFilenamePrefix, Value: constants.BackupTypePreUpgrade},
		{Name: constants.EnvBackupUsePathStyle, Value: fmt.Sprintf("%t", backupCfg.Target.UsePathStyle)},
	}

	// Add credentials secret reference if provided
	// SECURITY: Do NOT pass cross-namespace references. Secrets must be in cluster.Namespace.
	if backupCfg.Target.CredentialsSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupCredentialsSecretName,
			Value: backupCfg.Target.CredentialsSecretRef.Name,
		})
		// Namespace is always cluster.Namespace - cross-namespace references are not allowed
	}

	// Add JWT Auth configuration (preferred method)
	if backupCfg.JWTAuthRole != "" {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupJWTAuthRole,
			Value: backupCfg.JWTAuthRole,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupAuthMethod,
			Value: constants.BackupAuthMethodJWT,
		})
	}

	// Add token secret reference if provided (fallback for token-based auth)
	// SECURITY: Do NOT pass cross-namespace references. Secrets must be in cluster.Namespace.
	if backupCfg.TokenSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name:  constants.EnvBackupTokenSecretName,
			Value: backupCfg.TokenSecretRef.Name,
		})
		// Namespace is always cluster.Namespace - cross-namespace references are not allowed
		// Only set auth method to token if JWT Auth is not configured
		if backupCfg.JWTAuthRole == "" {
			env = append(env, corev1.EnvVar{
				Name:  constants.EnvBackupAuthMethod,
				Value: constants.BackupAuthMethodToken,
			})
		}
	}

	backoffLimit := int32(0)               // Don't retry failed backups automatically
	ttlSecondsAfterFinished := int32(3600) // 1 hour TTL

	image := verifiedExecutorDigest
	if image == "" {
		image = strings.TrimSpace(backupCfg.ExecutorImage)
	}
	if image == "" {
		image = constants.DefaultBackupImage()
	}

	// Build volumes and volume mounts
	volumes := []corev1.Volume{
		{
			Name: "tls-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cluster.Name + constants.SuffixTLSCA,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tls-ca",
			MountPath: constants.PathTLS,
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
			MountPath: constants.PathBackupCredentials,
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
	backupServiceAccountName := cluster.Name + constants.SuffixBackupServiceAccount

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:           constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:       cluster.Name,
				constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:    cluster.Name,
				constants.LabelOpenBaoComponent:  backup.ComponentBackup,
				constants.LabelOpenBaoBackupType: "pre-upgrade",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.LabelAppName:           constants.LabelValueAppNameOpenBao,
						constants.LabelAppInstance:       cluster.Name,
						constants.LabelAppManagedBy:      constants.LabelValueAppManagedByOpenBaoOperator,
						constants.LabelOpenBaoCluster:    cluster.Name,
						constants.LabelOpenBaoComponent:  backup.ComponentBackup,
						constants.LabelOpenBaoBackupType: constants.BackupTypePreUpgrade,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: backupServiceAccountName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(constants.UserBackup),
						RunAsGroup:   ptr.To(constants.GroupBackup),
						FSGroup:      ptr.To(constants.GroupBackup),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:         backup.ComponentBackup,
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
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

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
	if err := m.client.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status after initializing upgrade: %w", err)
	}

	logger.Info("Upgrade initialized; StatefulSet partition locked",
		"partition", cluster.Spec.Replicas)

	return nil
}

// performPodByPodUpgrade executes the rolling update, one pod at a time.
// Returns true when all pods have been upgraded.
// Returns false with nil error when waiting for a condition (caller should requeue).
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

	leaderPodName, err := m.currentLeaderPodByLabel(ctx, cluster)
	if err != nil {
		logger.Info("Unable to determine current leader from pod labels; attempting safe step-down", "error", err)
	}

	// Step-down leader if needed (level-triggered)
	if leaderPodName == "" || leaderPodName == podName {
		logger.Info("Initiating leader step-down before updating pod", "pod", podName, "currentLeader", leaderPodName)
		stepDownComplete, err := m.stepDownLeader(ctx, logger, cluster, podName, metrics)
		if err != nil {
			return false, err
		}
		if !stepDownComplete {
			// Step-down in progress, requeue
			return false, nil
		}
	}

	// Decrement partition to allow this pod to update
	newPartition := currentPartition - 1
	if err := m.setStatefulSetPartition(ctx, cluster, newPartition); err != nil {
		return false, fmt.Errorf("failed to update partition: %w", err)
	}

	// Check pod readiness (level-triggered)
	podReady, err := m.waitForPodReady(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !podReady {
		// Pod not ready yet, requeue
		return false, nil
	}

	// Check pod health (level-triggered)
	podHealthy, err := m.waitForPodHealthy(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !podHealthy {
		// Pod not healthy yet, requeue
		return false, nil
	}

	// Update progress
	SetUpgradeProgress(&cluster.Status, newPartition, targetOrdinal, cluster.Spec.Replicas, cluster.Generation)

	// Record pod upgrade duration
	podDuration := time.Since(podStartTime).Seconds()
	metrics.RecordPodDuration(podDuration, podName)
	metrics.SetPodsCompleted(len(cluster.Status.Upgrade.CompletedPods))
	metrics.SetPartition(newPartition)

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

// currentLeaderPodByLabel returns the pod name labeled as the current leader, if available.
// Returns an empty string if no leader label is observed.
func (m *Manager) currentLeaderPodByLabel(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	podList := &corev1.PodList{}
	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelAppInstance: cluster.Name,
			constants.LabelAppName:     constants.LabelValueAppNameOpenBao,
		}),
	); err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	leaders := make([]string, 0, 1)
	for i := range podList.Items {
		pod := &podList.Items[i]
		leader, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
		if err != nil || !present {
			continue
		}
		if leader {
			leaders = append(leaders, pod.Name)
		}
	}

	switch len(leaders) {
	case 0:
		return "", nil
	case 1:
		return leaders[0], nil
	default:
		return "", fmt.Errorf("multiple leaders detected via pod labels (%d)", len(leaders))
	}
}

// stepDownLeader performs a leader step-down check using level-triggered semantics.
// Instead of blocking with a ticker loop, it checks the condition once and returns
// a result indicating whether to requeue.
//
// Returns:
//   - (true, nil) if step-down is complete
//   - (false, nil) if step-down is in progress (caller should requeue)
//   - (false, error) if step-down failed fatally
func (m *Manager) stepDownLeader(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string, metrics *Metrics) (bool, error) {
	// Record step-down attempt (only on first call, tracked by LastStepDownTime)
	if cluster.Status.Upgrade == nil || cluster.Status.Upgrade.LastStepDownTime == nil {
		metrics.IncrementStepDownTotal()

		// Audit log: Leader step-down operation
		logging.LogAuditEvent(logger, "StepDown", map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
			"target_version":    cluster.Status.Upgrade.TargetVersion,
			"from_version":      cluster.Status.Upgrade.FromVersion,
		})
	}

	// Check if we've exceeded the timeout based on upgrade start time
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > DefaultStepDownTimeout {
			metrics.IncrementStepDownFailures()
			SetUpgradeFailed(&cluster.Status, ReasonStepDownTimeout,
				fmt.Sprintf(MessageStepDownTimeout, DefaultStepDownTimeout),
				cluster.Generation)
			return false, fmt.Errorf("step-down timeout: exceeded %v", DefaultStepDownTimeout)
		}
	}

	// Ensure step-down Job exists/is running
	result, err := ensureUpgradeExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		ExecutorActionRollingStepDownLeader,
		podName,
		"",
		"",
	)
	if err != nil {
		return false, fmt.Errorf("failed to ensure step-down Job: %w", err)
	}
	if result.Failed {
		metrics.IncrementStepDownFailures()
		return false, fmt.Errorf("step-down Job %s failed", result.Name)
	}
	if result.Running {
		logger.V(1).Info("Step-down job still running", "pod", podName)
		return false, nil // Requeue
	}

	// Job succeeded - check if leadership has transferred
	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      podName,
	}, pod); err != nil {
		logger.V(1).Info("Error getting pod after step-down", "error", err)
		return false, nil // Requeue
	}

	stillLeader, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
	if err != nil {
		logger.V(1).Info("Invalid OpenBao leader label value after step-down", "error", err)
		return false, nil // Requeue
	}

	if present && !stillLeader {
		logger.Info("Leadership transferred successfully", "previousLeader", podName)
		SetStepDownPerformed(&cluster.Status)
		logging.LogAuditEvent(logger, "StepDownCompleted", map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
		})
		return true, nil
	}

	// If the previous leader label is missing, treat transfer as complete if we
	// can observe a different pod labeled as leader.
	if !present {
		podList := &corev1.PodList{}
		if err := m.client.List(ctx, podList,
			client.InNamespace(cluster.Namespace),
			client.MatchingLabels(map[string]string{
				constants.LabelAppInstance: cluster.Name,
				constants.LabelAppName:     constants.LabelValueAppNameOpenBao,
			}),
		); err != nil {
			logger.V(1).Info("Error listing pods while waiting for leader transfer", "error", err)
			return false, nil // Requeue
		}

		for i := range podList.Items {
			candidate := &podList.Items[i]
			if candidate.Name == podName {
				continue
			}
			leader, leaderPresent, err := openbaoapi.ParseBoolLabel(candidate.Labels, openbaoapi.LabelActive)
			if err != nil || !leaderPresent {
				continue
			}
			if leader {
				logger.Info("Leadership transferred successfully", "previousLeader", podName, "newLeader", candidate.Name)
				SetStepDownPerformed(&cluster.Status)
				logging.LogAuditEvent(logger, "StepDownCompleted", map[string]string{
					"cluster_namespace": cluster.Namespace,
					"cluster_name":      cluster.Name,
					"pod":               podName,
				})
				return true, nil
			}
		}
	}

	logger.V(1).Info("Waiting for leadership transfer", "pod", podName)
	return false, nil // Requeue
}

// setStatefulSetPartition updates the StatefulSet's partition value using Server-Side Apply
// with a distinct field manager to avoid conflicts with InfraManager's SSA operations.
// This ensures the partition field is owned by the upgrade manager and won't be overridden
// by InfraManager's reconciliation loop.
func (m *Manager) setStatefulSetPartition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, partition int32) error {
	sts := &appsv1.StatefulSet{}
	stsName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	if err := m.client.Get(ctx, stsName, sts); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Create a patch that only updates the partition field.
	// We use client.MergeFrom instead of Server-Side Apply (SSA) to avoid conflicts
	// with the infra manager which owns the rest of the StatefulSet spec.
	// This generates a strategic merge patch that only touches the modified fields,
	// preventing "zero value" overwrites and "stale cache" issues.
	newSts := sts.DeepCopy()
	newSts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	newSts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: &partition,
	}

	// Patch using MergeFrom to send only the differences
	if err := m.client.Patch(ctx, newSts, client.MergeFrom(sts)); err != nil {
		return fmt.Errorf("failed to update StatefulSet partition: %w", err)
	}

	return nil
}

// waitForPodReady checks if a pod is Ready using level-triggered semantics.
// Instead of blocking, it checks the condition once and returns the result.
//
// Returns:
//   - (true, nil) if pod is ready
//   - (false, nil) if pod is not ready yet (caller should requeue)
//   - (false, error) if timeout exceeded or fatal error
func (m *Manager) waitForPodReady(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	// Check timeout based on when the upgrade started
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > DefaultPodReadyTimeout {
			SetUpgradeFailed(&cluster.Status, ReasonPodNotReady,
				fmt.Sprintf(MessagePodNotReady, podName, DefaultPodReadyTimeout),
				cluster.Generation)
			return false, fmt.Errorf("pod %s did not become ready within %v", podName, DefaultPodReadyTimeout)
		}
	}

	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      podName,
	}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Pod not found yet; waiting", "pod", podName)
			return false, nil // Requeue
		}
		return false, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	if isPodReady(pod) {
		logger.Info("Pod is ready", "pod", podName)
		return true, nil
	}

	logger.V(1).Info("Waiting for pod to become ready", "pod", podName, "phase", pod.Status.Phase)
	return false, nil // Requeue
}

// waitForPodHealthy checks if OpenBao is healthy on a pod using level-triggered semantics.
// Instead of blocking, it checks the condition once and returns the result.
//
// Returns:
//   - (true, nil) if pod is healthy
//   - (false, nil) if pod is not healthy yet (caller should requeue)
//   - (false, error) if timeout exceeded or fatal error
func (m *Manager) waitForPodHealthy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	// Check timeout based on when the upgrade started - health check should complete
	// within a reasonable window after the pod becomes ready
	// We use DefaultPodReadyTimeout + DefaultHealthCheckTimeout as total budget
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > DefaultPodReadyTimeout+DefaultHealthCheckTimeout {
			SetUpgradeFailed(&cluster.Status, ReasonHealthCheckFailed,
				fmt.Sprintf(MessageHealthCheckFailed, podName, "timeout"),
				cluster.Generation)
			return false, fmt.Errorf("OpenBao health check timeout for pod %s", podName)
		}
	}

	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		// CA cert not available yet, requeue
		logger.V(1).Info("CA certificate not available yet", "error", err)
		return false, nil // Requeue
	}

	podURL := m.getPodURL(cluster, podName)
	apiClient, err := m.clientFactory(openbaoapi.ClientConfig{
		ClusterKey: fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name),
		BaseURL:    podURL,
		CACert:     caCert,
	})
	if err != nil {
		// Wrap connection errors as transient and requeue
		if operatorerrors.IsTransientConnection(err) {
			logger.V(1).Info("Transient connection error creating client", "error", err)
			return false, nil // Requeue
		}
		return false, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	healthy, err := apiClient.IsHealthy(ctx)
	if err != nil {
		logger.V(1).Info("Health check error; will retry", "pod", podName, "error", err)
		return false, nil // Requeue
	}
	if healthy {
		logger.Info("OpenBao is healthy on pod", "pod", podName)
		return true, nil
	}

	logger.V(1).Info("Waiting for OpenBao to become healthy", "pod", podName)
	return false, nil // Requeue
}

// finalizeUpgrade completes the upgrade process.
func (m *Manager) finalizeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *Metrics) error {
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

	var upgradeDuration float64
	var fromVersion string

	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		upgradeDuration = time.Since(cluster.Status.Upgrade.StartedAt.Time).Seconds()
		fromVersion = cluster.Status.Upgrade.FromVersion
	}

	// Mark upgrade complete
	SetUpgradeComplete(&cluster.Status, cluster.Spec.Version, cluster.Generation)

	// Update status
	if err := m.client.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
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
	metrics.SetPartition(0)

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
		constants.LabelAppInstance:  cluster.Name,
		constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	})

	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, err
	}

	// Filter out backup job pods and other non-StatefulSet pods
	// StatefulSet pods have a name pattern: <cluster-name>-<ordinal>
	// Backup job pods have labels like "openbao.org/component": backup.ComponentBackup
	filteredPods := make([]corev1.Pod, 0, len(podList.Items))
	statefulSetPrefix := cluster.Name + "-"
	for _, pod := range podList.Items {
		// Skip backup job pods (they have the backup component label)
		if pod.Labels[constants.LabelOpenBaoComponent] == backup.ComponentBackup {
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
	secretName := cluster.Name + constants.SuffixTLSCA
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

// applyResource uses Server-Side Apply to create or update a Kubernetes resource.
// This eliminates the need for Get-then-Create-or-Update logic and manual diffing.
//
// The resource must have TypeMeta, ObjectMeta (with Name and Namespace), and the desired Spec set.
// Owner references are set automatically if the resource supports them.
//
// fieldOwner identifies the operator as the manager of this resource (used for conflict resolution).
func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster, fieldOwner string) error {
	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(cluster, obj, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Use Server-Side Apply with ForceOwnership to ensure the operator manages this resource
	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(fieldOwner),
	}

	if err := m.client.Patch(ctx, obj, client.Apply, patchOpts...); err != nil {
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

// ensureUpgradeServiceAccount creates or updates the ServiceAccount for upgrade operations using Server-Side Apply.
// This ServiceAccount is used for JWT Auth authentication to OpenBao.
func (m *Manager) ensureUpgradeServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := upgradeServiceAccountName(cluster)

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:      cluster.Name,
				constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:   cluster.Name,
				constants.LabelOpenBaoComponent: "upgrade",
			},
		},
	}

	if err := m.applyResource(ctx, sa, cluster, "openbao-operator"); err != nil {
		return fmt.Errorf("failed to ensure upgrade ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// upgradeServiceAccountName returns the name for the upgrade ServiceAccount.
func upgradeServiceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixUpgradeServiceAccount
}

// getPodURL returns the URL for connecting to a specific pod.
func (m *Manager) getPodURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	// Use the pod's direct DNS name for the headless service
	// Format: <pod-name>.<service-name>.<namespace>.svc:<port>
	serviceName := cluster.Name + headlessServiceSuffix
	return fmt.Sprintf("https://%s.%s.%s.svc:%d",
		podName, serviceName, cluster.Namespace, constants.PortAPI)
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
