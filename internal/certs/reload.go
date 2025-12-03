package certs

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

const (
	// tlsCertHashAnnotation is the annotation key used to track the current TLS certificate hash
	tlsCertHashAnnotation = "openbao.org/tls-cert-hash"
)

// KubernetesReloadSignaler implements ReloadSignaler by annotating OpenBao pods
// with the active TLS certificate hash. A dedicated in-pod sidecar can watch
// this annotation or the mounted TLS volume and send SIGHUP to the OpenBao
// process when changes are detected, avoiding the need for pods/exec from the
// operator itself.
type KubernetesReloadSignaler struct {
	clientset kubernetes.Interface
}

// NewKubernetesReloadSignaler creates a new KubernetesReloadSignaler.
func NewKubernetesReloadSignaler(clientset kubernetes.Interface) *KubernetesReloadSignaler {
	return &KubernetesReloadSignaler{
		clientset: clientset,
	}
}

// SignalReload annotates all ready OpenBao pods in the cluster with the current
// TLS certificate hash. Implementations running inside the pod (for example,
// a sidecar container) may watch this annotation or the underlying volume and
// send SIGHUP to the OpenBao process when a new hash is observed.
func (k *KubernetesReloadSignaler) SignalReload(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, certHash string) error {
	// List all pods for this cluster using the same labels as the StatefulSet
	podList, err := k.clientset.CoreV1().Pods(cluster.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			"app.kubernetes.io/instance":   cluster.Name,
			"app.kubernetes.io/name":       "openbao",
			"app.kubernetes.io/managed-by": "openbao-operator",
		}).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	if len(podList.Items) == 0 {
		logger.Info("No pods found for OpenBaoCluster; skipping TLS reload signal")
		return nil
	}

	var lastErr error
	updatedCount := 0

	for i := range podList.Items {
		pod := &podList.Items[i]

		// Check if pod is ready
		if !isPodReady(pod) {
			logger.Info("Skipping TLS reload for pod that is not ready", "pod", pod.Name)
			continue
		}

		// Check if we've already signaled this hash
		currentHash := pod.Annotations[tlsCertHashAnnotation]
		if currentHash == certHash {
			logger.V(1).Info("Pod already has the current certificate hash; skipping", "pod", pod.Name, "hash", certHash)
			continue
		}

		// Update pod annotation with the new hash
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		pod.Annotations[tlsCertHashAnnotation] = certHash

		if _, err := k.clientset.CoreV1().Pods(pod.Namespace).Update(ctx, pod, metav1.UpdateOptions{}); err != nil {
			logger.Error(err, "Failed to update pod annotation with certificate hash", "pod", pod.Name)
			lastErr = err
			continue
		}

		updatedCount++
		logger.Info("Marked pod for TLS reload via certificate hash annotation", "pod", pod.Name, "hash", certHash)
	}

	if updatedCount == 0 && lastErr == nil {
		logger.Info("No pods required TLS reload annotation update", "totalPods", len(podList.Items))
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("failed to annotate some pods for TLS reload (updated %d/%d): %w", updatedCount, len(podList.Items), lastErr)
	}

	logger.Info("Successfully annotated pods for TLS reload", "updatedCount", updatedCount, "totalPods", len(podList.Items))
	return nil
}

// isPodReady checks if a pod is in Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
