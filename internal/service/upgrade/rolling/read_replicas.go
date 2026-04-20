package rolling

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func (m *Manager) ensureReadReplicaPoolReadyForRollingUpgrade(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (recon.Result, bool, error) {
	desiredReplicas := desiredReadReplicaReplicas(cluster)
	if desiredReplicas == 0 {
		return recon.Result{}, false, nil
	}

	sts, err := m.getReadReplicaStatefulSet(ctx, cluster)
	if err != nil {
		return recon.Result{}, false, err
	}

	if sts.Generation > 0 && sts.Status.ObservedGeneration < sts.Generation {
		logger.Info("Waiting for read-replica StatefulSet generation to be observed before rolling voter upgrade",
			"statefulSet", sts.Name,
			"generation", sts.Generation,
			"observedGeneration", sts.Status.ObservedGeneration)
		return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
	}

	if sts.Status.ReadyReplicas != desiredReplicas ||
		sts.Status.UpdatedReplicas != desiredReplicas ||
		sts.Status.CurrentRevision == "" ||
		sts.Status.CurrentRevision != sts.Status.UpdateRevision {
		logger.Info("Waiting for read-replica StatefulSet convergence before rolling voter upgrade",
			"statefulSet", sts.Name,
			"readyReplicas", sts.Status.ReadyReplicas,
			"updatedReplicas", sts.Status.UpdatedReplicas,
			"desiredReplicas", desiredReplicas,
			"currentRevision", sts.Status.CurrentRevision,
			"updateRevision", sts.Status.UpdateRevision)
		return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
	}

	pods, err := m.getReadReplicaPods(ctx, cluster)
	if err != nil {
		return recon.Result{}, false, err
	}
	if len(pods) != int(desiredReplicas) {
		logger.Info("Waiting for expected number of read-replica pods before rolling voter upgrade",
			"foundPods", len(pods),
			"desiredReplicas", desiredReplicas)
		return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
	}

	for i := range pods {
		pod := &pods[i]
		if !isPodReady(pod) {
			logger.Info("Waiting for read-replica pod readiness before rolling voter upgrade",
				"pod", pod.Name,
				"phase", pod.Status.Phase)
			return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
		}
		if pod.Labels[appsv1.StatefulSetRevisionLabel] != sts.Status.UpdateRevision {
			logger.Info("Waiting for read-replica pod revision convergence before rolling voter upgrade",
				"pod", pod.Name,
				"podRevision", pod.Labels[appsv1.StatefulSetRevisionLabel],
				"targetRevision", sts.Status.UpdateRevision)
			return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
		}

		healthy, err := m.isPodHealthyForFinalization(ctx, logger, cluster, pod.Name)
		if err != nil {
			return recon.Result{}, false, err
		}
		if !healthy {
			logger.Info("Waiting for read-replica pod health before rolling voter upgrade", "pod", pod.Name)
			return recon.Result{RequeueAfter: constants.RequeueShort}, true, nil
		}
	}

	return recon.Result{}, false, nil
}

func desiredReadReplicaReplicas(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas <= 0 {
		return 0
	}
	return cluster.Spec.ReadReplicas.Replicas
}

func (m *Manager) getReadReplicaStatefulSet(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*appsv1.StatefulSet, error) {
	sts := &appsv1.StatefulSet{}
	stsKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ReadReplicaStatefulSetName(cluster),
	}
	if err := m.client.Get(ctx, stsKey, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("read-replica StatefulSet not found while preparing rolling upgrade")
		}
		return nil, fmt.Errorf("failed to get read-replica StatefulSet: %w", err)
	}
	return sts, nil
}

func (m *Manager) getReadReplicaPods(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(resourceidentity.ReadReplicaPodSelectorLabels(cluster))
	if err := m.client.List(ctx, podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		return nil, fmt.Errorf("failed to list read-replica pods: %w", err)
	}
	return podList.Items, nil
}
