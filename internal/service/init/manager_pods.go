package init

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func isPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// findFirstPod finds the first pod (pod-0) for the given cluster.
// During initial cluster creation, this should be the only pod.
func (m *Manager) findFirstPod(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*corev1.Pod, error) {
	podList, err := m.clientset.CoreV1().Pods(cluster.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			constants.LabelAppInstance:  cluster.Name,
			constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
			constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
		}).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		if strings.HasSuffix(pod.Name, "-0") {
			return pod, nil
		}
	}

	return nil, nil
}

func logContainerNotReady(logger logr.Logger, pod *corev1.Pod) {
	if len(pod.Status.ContainerStatuses) > 0 {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == constants.ContainerBao {
				if status.Started != nil {
					logger.V(1).Info("Container running but startup probe not passed yet; waiting", "pod", pod.Name, "phase", pod.Status.Phase, "started", *status.Started)
				} else {
					logger.V(1).Info("Container running but startup probe status not available yet; waiting", "pod", pod.Name, "phase", pod.Status.Phase)
				}
			}
		}
		return
	}

	logger.V(1).Info("Container status not yet populated; waiting for Kubernetes to update pod status", "pod", pod.Name, "phase", pod.Status.Phase)
}

// isContainerRunning checks if the OpenBao container is running.
// This is used instead of isPodReady because the readiness probe may fail
// until OpenBao is initialized, creating a chicken-and-egg problem.
// If the container has a startup probe, we wait for it to pass (status.Started == true)
// to ensure the service is actually listening before attempting initialization.
// Returns false if ContainerStatuses is nil or empty (pod status not yet populated by Kubernetes).
func isContainerRunning(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	if len(pod.Status.ContainerStatuses) == 0 {
		return false
	}

	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == constants.ContainerBao {
			if status.State.Running == nil {
				return false
			}
			if status.Started != nil {
				return *status.Started
			}
			return false
		}
	}

	return false
}
