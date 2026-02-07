package rolling

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/logging"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// performPodByPodUpgrade executes the rolling update, one pod at a time.
// Returns true when all pods have been upgraded.
// Returns false with nil error when waiting for a condition (caller should requeue).
func (m *Manager) performPodByPodUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *upgrade.Metrics) (bool, error) {
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

	// Check that the target pod has actually rolled to StatefulSet UpdateRevision.
	revisionUpdated, err := m.waitForPodRevisionUpdated(ctx, logger, cluster, podName)
	if err != nil {
		return false, err
	}
	if !revisionUpdated {
		return false, nil // Requeue
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
	upgrade.SetUpgradeProgress(&cluster.Status, newPartition, targetOrdinal)

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
func (m *Manager) stepDownLeader(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string, metrics *upgrade.Metrics) (bool, error) {
	if cluster.Status.Upgrade == nil {
		return false, fmt.Errorf("upgrade state is nil")
	}

	jobName := upgrade.ExecutorJobName(cluster.Name, upgrade.ExecutorActionRollingStepDownLeader, podName, "", "")
	jobKey := types.NamespacedName{Namespace: cluster.Namespace, Name: jobName}

	stepDownJob := &batchv1.Job{}
	jobExists := true
	if err := m.client.Get(ctx, jobKey, stepDownJob); err != nil {
		if apierrors.IsNotFound(err) {
			jobExists = false
		} else {
			return false, fmt.Errorf("failed to get step-down Job %s/%s: %w", cluster.Namespace, jobName, err)
		}
	}

	// Record step-down attempt only once per pod by keying off Job existence.
	// This avoids inflating metrics and audit events during requeues.
	if !jobExists {
		targetIsLeader, err := m.isTargetPodLeader(ctx, cluster, podName)
		if err != nil {
			logger.V(1).Info("Unable to confirm target pod leadership before step-down; will retry", "pod", podName, "error", err)
			return false, nil // Requeue
		}
		if !targetIsLeader {
			logger.V(1).Info("Skipping leader step-down because target pod is not leader", "pod", podName)
			return true, nil
		}

		metrics.IncrementStepDownTotal()

		// Audit log: Leader step-down operation
		logging.LogAuditEvent(logger, logging.EventStepDownStarted, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
			"target_version":    cluster.Status.Upgrade.TargetVersion,
			"from_version":      cluster.Status.Upgrade.FromVersion,
		})
	}

	// Ensure step-down Job exists/is running
	result, err := upgrade.EnsureExecutorJob(
		ctx,
		m.client,
		m.scheme,
		logger,
		cluster,
		upgrade.ExecutorActionRollingStepDownLeader,
		podName,
		"",
		"",
		m.clientConfig,
		m.operatorImageVerifier,
		m.Platform,
	)
	if err != nil {
		return false, fmt.Errorf("failed to ensure step-down Job: %w", err)
	}
	if result.Failed {
		metrics.IncrementStepDownFailures()
		return false, fmt.Errorf("step-down Job %s failed", result.Name)
	}
	if result.Running {
		// Time out only while the step-down Job is actively running.
		// A completed Job may still be observed on subsequent reconciles while
		// pod labels/API leadership settle; that must not be treated as timeout.
		if jobExists && !stepDownJob.CreationTimestamp.IsZero() {
			elapsed := time.Since(stepDownJob.CreationTimestamp.Time)
			if elapsed > upgrade.DefaultStepDownTimeout {
				metrics.IncrementStepDownFailures()
				upgrade.SetUpgradeFailed(
					&cluster.Status,
					upgrade.ReasonStepDownTimeout,
					fmt.Sprintf(upgrade.MessageStepDownTimeout, podName),
				)
				return false, fmt.Errorf("step-down timeout for pod %s: exceeded %v", podName, upgrade.DefaultStepDownTimeout)
			}
		}
		logger.V(1).Info("Step-down job still running", "pod", podName)
		return false, nil // Requeue
	}

	// Job succeeded - check if the pod we're about to restart is no longer leader.
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
		upgrade.SetStepDownPerformed(&cluster.Status)
		logging.LogAuditEvent(logger, logging.EventStepDownCompleted, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
		})
		return true, nil
	}

	// Labels can lag behind reality; confirm leadership via the OpenBao API.
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		logger.V(1).Info("CA certificate not available yet while checking leader transfer", "error", err)
		return false, nil // Requeue
	}

	apiClient, err := m.newPodClient(cluster, podName, caCert)
	if err != nil {
		if operatorerrors.IsTransientConnection(err) {
			logger.V(1).Info("Transient connection error creating client while checking leader transfer", "error", err)
			return false, nil // Requeue
		}
		return false, fmt.Errorf("failed to create OpenBao client while checking leader transfer: %w", err)
	}

	isLeader, err := apiClient.IsLeader(ctx)
	if err != nil {
		logger.V(1).Info("Leader check failed after step-down; will retry", "pod", podName, "error", err)
		return false, nil // Requeue
	}
	if !isLeader {
		logger.Info("Leadership transferred successfully (verified via API)", "previousLeader", podName)
		upgrade.SetStepDownPerformed(&cluster.Status)
		logging.LogAuditEvent(logger, logging.EventStepDownCompleted, map[string]string{
			"cluster_namespace": cluster.Namespace,
			"cluster_name":      cluster.Name,
			"pod":               podName,
		})
		return true, nil
	}

	// The step-down Job can succeed while leadership quickly returns to the same pod.
	// If that persisted longer than the step-down timeout window, recycle the Job so
	// the next reconcile performs another step-down attempt for this pod.
	if jobExists && !stepDownJob.CreationTimestamp.IsZero() {
		elapsed := time.Since(stepDownJob.CreationTimestamp.Time)
		if elapsed > upgrade.DefaultStepDownTimeout {
			// Use foreground deletion so the old Job pod is removed before a new
			// retry Job with the same deterministic name is created.
			propagationPolicy := metav1.DeletePropagationForeground
			if err := m.client.Delete(
				ctx,
				stepDownJob,
				&client.DeleteOptions{PropagationPolicy: &propagationPolicy},
			); err != nil && !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to delete stale step-down Job %s/%s for retry: %w", cluster.Namespace, stepDownJob.Name, err)
			}
			logger.Info("Step-down job succeeded but target pod is still leader; deleting job to retry",
				"pod", podName,
				"job", stepDownJob.Name,
				"elapsed", elapsed)
			return false, nil
		}
	}

	logger.V(1).Info("Waiting for leadership transfer", "pod", podName)
	return false, nil // Requeue
}

func (m *Manager) isTargetPodLeader(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to get CA certificate for leader check: %w", err)
	}

	apiClient, err := m.newPodClient(cluster, podName, caCert)
	if err != nil {
		if operatorerrors.IsTransientConnection(err) {
			return false, err
		}
		return false, fmt.Errorf("failed to create OpenBao client for leader check: %w", err)
	}

	isLeader, err := apiClient.IsLeader(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check leadership for pod %s: %w", podName, err)
	}

	return isLeader, nil
}

// setStatefulSetPartition updates the StatefulSet's partition value using strategic merge patch.
// We use MergeFrom instead of SSA because StatefulSet validation requires all required fields
// (selector, serviceName, template labels) to be present in SSA patches, but MergeFrom only
// sends the diff and doesn't have this limitation.
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
	// We use client.MergeFrom instead of Server-Side Apply (SSA) because SSA requires
	// all required StatefulSet fields (selector, serviceName, template labels) to be present,
	// which causes validation errors. MergeFrom generates a strategic merge patch that only
	// touches the modified fields.
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

// waitForPodRevisionUpdated checks whether a pod has rolled to the StatefulSet update revision.
// Returns:
//   - (true, nil) if the pod revision matches StatefulSet UpdateRevision
//   - (false, nil) if not updated yet (caller should requeue)
//   - (false, error) if timeout exceeded or fatal error
func (m *Manager) waitForPodRevisionUpdated(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		elapsed := time.Since(cluster.Status.Upgrade.StartedAt.Time)
		if elapsed > upgrade.DefaultPodReadyTimeout {
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonPodNotReady,
				fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout))
			return false, fmt.Errorf("pod %s did not roll to update revision within %v", podName, upgrade.DefaultPodReadyTimeout)
		}
	}

	sts := &appsv1.StatefulSet{}
	stsKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, stsKey, sts); err != nil {
		return false, fmt.Errorf("failed to get StatefulSet while checking pod revision: %w", err)
	}

	targetRevision := sts.Status.UpdateRevision
	if targetRevision == "" {
		logger.V(1).Info("StatefulSet update revision not set yet; waiting")
		return false, nil
	}

	pod := &corev1.Pod{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      podName,
	}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Pod not found yet while waiting for revision update", "pod", podName)
			return false, nil
		}
		return false, fmt.Errorf("failed to get pod %s while checking revision: %w", podName, err)
	}

	podRevision := pod.Labels[appsv1.StatefulSetRevisionLabel]
	if podRevision != targetRevision {
		logger.V(1).Info("Waiting for pod revision update",
			"pod", podName,
			"currentRevision", podRevision,
			"targetRevision", targetRevision)
		return false, nil
	}

	logger.Info("Pod revision updated", "pod", podName, "revision", targetRevision)
	return true, nil
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
		if elapsed > upgrade.DefaultPodReadyTimeout {
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonPodNotReady,
				fmt.Sprintf(upgrade.MessagePodNotReady, podName, upgrade.DefaultPodReadyTimeout))
			return false, fmt.Errorf("pod %s did not become ready within %v", podName, upgrade.DefaultPodReadyTimeout)
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
		if elapsed > upgrade.DefaultPodReadyTimeout+upgrade.DefaultHealthCheckTimeout {
			upgrade.SetUpgradeFailed(&cluster.Status, upgrade.ReasonHealthCheckFailed,
				fmt.Sprintf(upgrade.MessageHealthCheckFailed, podName, "timeout"))
			return false, fmt.Errorf("OpenBao health check timeout for pod %s", podName)
		}
	}

	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		// CA cert not available yet, requeue
		logger.V(1).Info("CA certificate not available yet", "error", err)
		return false, nil // Requeue
	}

	apiClient, err := m.newPodClient(cluster, podName, caCert)
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
