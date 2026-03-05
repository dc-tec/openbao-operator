package rolling

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/logging"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// initializeUpgrade sets up the upgrade state and locks the StatefulSet partition.
func (m *Manager) initializeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *upgrade.Metrics, strategy string) error {
	fromVersion := cluster.Status.CurrentVersion
	toVersion := cluster.Spec.Version

	logger.Info("Initializing upgrade",
		"from", fromVersion,
		"to", toVersion,
		"replicas", cluster.Spec.Replicas)

	// Set upgrade state
	upgrade.SetUpgradeStarted(&cluster.Status, fromVersion, toVersion, cluster.Spec.Replicas)

	// Lock StatefulSet by setting partition to replicas (prevents all updates)
	if err := m.setStatefulSetPartition(ctx, cluster, cluster.Spec.Replicas); err != nil {
		return fmt.Errorf("failed to lock StatefulSet partition: %w", err)
	}

	// Update status using SSA
	if err := m.patchStatusSSA(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status after initializing upgrade: %w", err)
	}
	// Only increment after the upgrade start state has been persisted successfully.
	if metrics != nil {
		metrics.IncrementTotal(strategy)
	}
	logging.LogAuditEvent(logger, logging.EventUpgradeStarted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"strategy":          strategy,
		"from_version":      fromVersion,
		"to_version":        toVersion,
	})

	logger.Info("Upgrade initialized; StatefulSet partition locked",
		"partition", cluster.Spec.Replicas)

	return nil
}

// finalizeUpgrade completes the upgrade process.
func (m *Manager) finalizeUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *upgrade.Metrics, strategy string) error {
	var upgradeDuration float64
	var fromVersion string

	if cluster.Status.Upgrade != nil && cluster.Status.Upgrade.StartedAt != nil {
		upgradeDuration = time.Since(cluster.Status.Upgrade.StartedAt.Time).Seconds()
		fromVersion = cluster.Status.Upgrade.FromVersion
	}

	// Mark upgrade complete
	upgrade.SetUpgradeComplete(&cluster.Status, cluster.Spec.Version)

	if err := m.patchFinalizedUpgradeStatus(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status after completing upgrade: %w", err)
	}

	// Record metrics
	if upgradeDuration > 0 {
		metrics.RecordDuration(upgradeDuration, fromVersion, cluster.Spec.Version)
	}
	metrics.SetInProgress(false)
	metrics.SetStatus(upgrade.UpgradeStatusSuccess)
	metrics.IncrementSuccess(strategy)
	metrics.SetPodsCompleted(0)
	metrics.SetTotalPods(0)
	metrics.SetPartition(0)
	logging.LogAuditEvent(logger, logging.EventUpgradeCompleted, map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"strategy":          strategy,
		"version":           cluster.Spec.Version,
	})

	logger.Info("Upgrade completed successfully",
		"version", cluster.Spec.Version,
		"duration", upgradeDuration)

	return nil
}

func (m *Manager) patchFinalizedUpgradeStatus(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	latest := &openbaov1alpha1.OpenBaoCluster{}
	clusterKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, clusterKey, latest); err != nil {
		return fmt.Errorf("failed to get cluster for final status patch: %w", err)
	}

	desired := latest.DeepCopy()
	desired.Status.Upgrade = nil
	desired.Status.CurrentVersion = cluster.Spec.Version

	if err := m.client.Status().Patch(ctx, desired, client.MergeFrom(latest)); err != nil {
		return fmt.Errorf("failed to patch finalized rolling upgrade status: %w", err)
	}

	cluster.Status.Upgrade = nil
	cluster.Status.CurrentVersion = cluster.Spec.Version
	return nil
}

func (m *Manager) waitForFinalizationConverged(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	sts := &appsv1.StatefulSet{}
	stsKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := m.client.Get(ctx, stsKey, sts); err != nil {
		return false, fmt.Errorf("failed to get StatefulSet while checking upgrade convergence: %w", err)
	}

	partition := int32(0)
	if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
	}

	rolloutComplete := sts.Status.ReadyReplicas == cluster.Spec.Replicas &&
		sts.Status.UpdatedReplicas == cluster.Spec.Replicas &&
		sts.Status.CurrentRevision != "" &&
		sts.Status.CurrentRevision == sts.Status.UpdateRevision &&
		partition == 0
	if !rolloutComplete {
		logger.Info("Waiting for StatefulSet convergence before finalizing rolling upgrade",
			"readyReplicas", sts.Status.ReadyReplicas,
			"updatedReplicas", sts.Status.UpdatedReplicas,
			"desiredReplicas", cluster.Spec.Replicas,
			"currentRevision", sts.Status.CurrentRevision,
			"updateRevision", sts.Status.UpdateRevision,
			"partition", partition)
		return false, nil
	}

	pods, err := m.getClusterPods(ctx, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to list cluster pods while checking upgrade convergence: %w", err)
	}
	if len(pods) != int(cluster.Spec.Replicas) {
		logger.Info("Waiting for expected number of pods before finalizing rolling upgrade",
			"foundPods", len(pods),
			"desiredReplicas", cluster.Spec.Replicas)
		return false, nil
	}

	for i := range pods {
		pod := &pods[i]
		if !isPodReady(pod) {
			logger.Info("Waiting for pod readiness before finalizing rolling upgrade", "pod", pod.Name, "phase", pod.Status.Phase)
			return false, nil
		}

		if pod.Labels[appsv1.StatefulSetRevisionLabel] != sts.Status.UpdateRevision {
			logger.Info("Waiting for pod revision convergence before finalizing rolling upgrade",
				"pod", pod.Name,
				"podRevision", pod.Labels[appsv1.StatefulSetRevisionLabel],
				"targetRevision", sts.Status.UpdateRevision)
			return false, nil
		}

		healthy, err := m.isPodHealthyForFinalization(ctx, logger, cluster, pod.Name)
		if err != nil {
			return false, err
		}
		if !healthy {
			return false, nil
		}
	}

	return true, nil
}

func (m *Manager) isPodHealthyForFinalization(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (bool, error) {
	caCert, err := m.getClusterCACert(ctx, cluster)
	if err != nil {
		logger.V(1).Info("CA certificate not available yet while checking finalization health", "pod", podName, "error", err)
		return false, nil
	}

	apiClient, err := m.newPodClient(cluster, podName, caCert)
	if err != nil {
		if operatorerrors.IsTransientConnection(err) {
			logger.V(1).Info("Transient connection error creating client while checking finalization health", "pod", podName, "error", err)
			return false, nil
		}
		return false, fmt.Errorf("failed to create OpenBao client for finalization health check on pod %s: %w", podName, err)
	}

	healthy, err := apiClient.IsHealthy(ctx)
	if err != nil {
		logger.V(1).Info("Health check error while finalizing rolling upgrade; will retry", "pod", podName, "error", err)
		return false, nil
	}
	if !healthy {
		logger.V(1).Info("Waiting for pod health before finalizing rolling upgrade", "pod", podName)
		return false, nil
	}

	return true, nil
}
