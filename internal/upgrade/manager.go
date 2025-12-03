package upgrade

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/logging"
	openbaoapi "github.com/openbao/operator/internal/openbao"
)

const (
	// tlsCASecretSuffix is the suffix for the TLS CA secret.
	tlsCASecretSuffix = "-tls-ca"
	// rootTokenSecretSuffix is the suffix for the root token secret.
	rootTokenSecretSuffix = "-root-token"
	// rootTokenSecretKey is the key in the root token secret.
	rootTokenSecretKey = "token"
	// headlessServiceSuffix is appended to cluster name for the headless service.
	headlessServiceSuffix = ""
	// openBaoContainerPort is the API port.
	openBaoContainerPort = 8200
)

// OpenBaoClientFactory creates OpenBao API clients for connecting to cluster pods.
// This is primarily used for testing to inject mock clients.
type OpenBaoClientFactory func(config openbaoapi.ClientConfig) (*openbaoapi.Client, error)

// Manager reconciles version and Raft-aware upgrade behavior for an OpenBaoCluster.
type Manager struct {
	client        client.Client
	clientFactory OpenBaoClientFactory
}

// NewManager constructs a Manager that uses the provided Kubernetes client.
func NewManager(c client.Client) *Manager {
	return &Manager{
		client:        c,
		clientFactory: openbaoapi.NewClient,
	}
}

// NewManagerWithClientFactory constructs a Manager with a custom OpenBao client factory.
// This is primarily used for testing.
func NewManagerWithClientFactory(c client.Client, factory OpenBaoClientFactory) *Manager {
	return &Manager{
		client:        c,
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

	// Phase 3: Initialize Upgrade (if not resuming)
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

	// Phase 4: Pod-by-Pod Update
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

	// Phase 5: Finalization
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

	// Sort by ordinal (descending) for consistent processing order
	pods := podList.Items
	sort.Slice(pods, func(i, j int) bool {
		ordinalI := extractOrdinal(pods[i].Name)
		ordinalJ := extractOrdinal(pods[j].Name)
		return ordinalI > ordinalJ
	})

	return pods, nil
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
func (m *Manager) getOperatorToken(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	secretName := cluster.Name + rootTokenSecretSuffix
	secret := &corev1.Secret{}

	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secretName,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// For self-init clusters, there's no root token
			// Users must configure a dedicated operator token
			return "", fmt.Errorf("root token secret not found; for self-init clusters, configure an operator token via spec.selfInit.requests")
		}
		return "", fmt.Errorf("failed to get root token secret: %w", err)
	}

	token, ok := secret.Data[rootTokenSecretKey]
	if !ok {
		return "", fmt.Errorf("token key not found in secret %s", secretName)
	}

	return string(token), nil
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
