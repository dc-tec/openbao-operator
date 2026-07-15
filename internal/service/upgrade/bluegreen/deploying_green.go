package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func (m *Manager) ensureBlueClusterReadyForGreen(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	blueRevision string,
) (phaseOutcome, bool, error) {
	bluePods, err := m.getPodsByRevision(ctx, cluster, blueRevision)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("failed to get Blue pods: %w", err)
	}
	if len(bluePods) == 0 {
		logger.Info("No Blue pods found yet, waiting...")
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	if !bluePodsHaveExpectedRevision(logger, bluePods, blueRevision) {
		logger.Info("Blue pods missing revision label; waiting for InfraManager to update StatefulSet")
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	if !blueClusterReadyForGreen(bluePods) {
		logger.Info("Blue pods not ready/unsealed yet, waiting before creating Green StatefulSet")
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}

	return phaseOutcome{}, false, nil
}

func bluePodsHaveExpectedRevision(logger logr.Logger, bluePods []corev1.Pod, blueRevision string) bool {
	for _, pod := range bluePods {
		if blueRevision == "" && pod.Labels[constants.LabelOpenBaoRevision] == "" {
			continue
		}
		rev, present := pod.Labels[constants.LabelOpenBaoRevision]
		if present && rev == blueRevision {
			continue
		}
		logger.Info("Blue pod missing revision label, InfraManager will update it",
			"pod", pod.Name,
			"expectedRevision", blueRevision,
			"actualRevision", rev)
		return false
	}
	return true
}

func blueClusterReadyForGreen(bluePods []corev1.Pod) bool {
	for _, pod := range bluePods {
		if !isPodReady(&pod) {
			continue
		}
		sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
		if err == nil && present && !sealed {
			return true
		}
	}
	return false
}

func (m *Manager) ensureGreenStatefulSetReady(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	blueRevision string,
	greenRevision string,
) (phaseOutcome, bool, error) {
	greenStatefulSet, found, err := m.getGreenStatefulSet(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, true, err
	}
	if !found {
		outcome, err := m.createGreenStatefulSet(ctx, logger, cluster, blueRevision, greenRevision)
		return outcome, true, err
	}

	if !greenStatefulSetReady(logger, cluster.Spec.Replicas, greenStatefulSet) {
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}

	outcome, waiting, err := m.waitForGreenPodsReady(ctx, logger, cluster, greenRevision)
	if waiting || err != nil {
		return outcome, true, err
	}

	return phaseOutcome{}, false, nil
}

func (m *Manager) getGreenStatefulSet(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	greenRevision string,
) (*appsv1.StatefulSet, bool, error) {
	greenStatefulSetName := fmt.Sprintf("%s-%s", cluster.Name, greenRevision)
	greenStatefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      greenStatefulSetName,
	}, greenStatefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get Green StatefulSet: %w", err)
	}

	return greenStatefulSet, true, nil
}

func greenStatefulSetReady(logger logr.Logger, desiredReplicas int32, greenStatefulSet *appsv1.StatefulSet) bool {
	if greenStatefulSet.Spec.Replicas != nil {
		desiredReplicas = *greenStatefulSet.Spec.Replicas
	}
	if greenStatefulSet.Status.ReadyReplicas < desiredReplicas {
		logger.Info("Waiting for Green pods to be ready",
			"readyReplicas", greenStatefulSet.Status.ReadyReplicas,
			"desiredReplicas", desiredReplicas)
		return false
	}
	return true
}

func (m *Manager) waitForGreenPodsReady(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	greenRevision string,
) (phaseOutcome, bool, error) {
	greenPods, err := m.getPodsByRevision(ctx, cluster, greenRevision)
	if err != nil {
		return phaseOutcome{}, true, fmt.Errorf("failed to get Green pods: %w", err)
	}
	for _, pod := range greenPods {
		if pod.Status.Phase == corev1.PodRunning {
			continue
		}
		logger.Info("Green pod not yet running", "pod", pod.Name, "phase", pod.Status.Phase)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}
	for _, pod := range greenPods {
		sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
		if err != nil {
			return phaseOutcome{}, true, fmt.Errorf("failed to parse sealed label on pod %s: %w", pod.Name, err)
		}
		if present && !sealed {
			continue
		}
		logger.Info("Waiting for Green pod to be unsealed", "pod", pod.Name)
		return requeueAfterOutcome(constants.RequeueShort), true, nil
	}

	return phaseOutcome{}, false, nil
}
