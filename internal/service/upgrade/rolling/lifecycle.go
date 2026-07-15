package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

func (m *Manager) patchFinalizedUpgradeStatus(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if m.adminOpsMutator == nil {
		return fmt.Errorf("adminops status mutator is required")
	}

	if err := m.adminOpsMutator(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.Upgrade = nil
		return nil
	}, true); err != nil {
		return fmt.Errorf("failed to patch finalized rolling upgrade status: %w", err)
	}

	return nil
}

// patchStatusSSA updates the cluster status using Server-Side Apply.
// SSA eliminates race conditions by having the API server merge changes,
// rather than requiring the client to refresh and merge manually.
func (m *Manager) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if m.adminOpsMutator == nil {
		return fmt.Errorf("adminops status mutator is required")
	}

	if err := m.adminOpsMutator(ctx, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.Upgrade = cluster.Status.Upgrade
		obj.Status.UpgradeRequests = cluster.Status.UpgradeRequests
		return nil
	}, false); err != nil {
		return fmt.Errorf("failed to apply adminops status plane for rolling upgrade status: %w", err)
	}

	return nil
}

func (m *Manager) waitForFinalizationConverged(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	sts := &appsv1.StatefulSet{}
	stsKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      upgrade.StableVoterStatefulSetName(cluster),
	}
	if err := m.client.Get(ctx, stsKey, sts); err != nil {
		return false, fmt.Errorf("failed to get StatefulSet while checking upgrade convergence: %w", err)
	}

	partition := int32(0)
	if sts.Spec.UpdateStrategy.RollingUpdate != nil && sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
	}

	statefulSetRevisionsConverged := sts.Status.ReadyReplicas == cluster.Spec.Replicas &&
		sts.Status.UpdatedReplicas == cluster.Spec.Replicas &&
		sts.Status.CurrentRevision != "" &&
		sts.Status.CurrentRevision == sts.Status.UpdateRevision
	if statefulSetRevisionsConverged && partition != 0 && cluster.Status.Upgrade != nil && cluster.Status.Upgrade.CurrentPartition == 0 {
		logger.Info("Repairing stale StatefulSet partition before finalizing rolling upgrade",
			"partition", partition)
		if err := m.setStatefulSetPartition(ctx, cluster, 0); err != nil {
			return false, fmt.Errorf("failed to repair stale StatefulSet partition before finalization: %w", err)
		}
		return false, nil
	}

	rolloutComplete := statefulSetRevisionsConverged && partition == 0
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
	caCert, err := raftops.LoadClusterCACert(ctx, m.client, cluster)
	if err != nil {
		logger.V(1).Info("CA certificate not available yet while checking finalization health", "pod", podName, "error", err)
		return false, nil
	}

	apiClient, err := raftops.NewClusterPodClient(cluster, podName, caCert, m.clientFactory, raftops.ClusterPodClientOptions{})
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
