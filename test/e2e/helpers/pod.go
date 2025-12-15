package helpers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodResult captures the final state and logs for a one-shot Pod.
type PodResult struct {
	Namespace string
	Name      string
	Phase     corev1.PodPhase
	Logs      string
}

// RunPodUntilCompletion creates the given Pod and waits until it completes
// (Succeeded or Failed), then returns its logs.
func RunPodUntilCompletion(
	ctx context.Context,
	cfg *rest.Config,
	c client.Client,
	pod *corev1.Pod,
	timeout time.Duration,
) (*PodResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rest config is required")
	}
	if c == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}
	if pod == nil {
		return nil, fmt.Errorf("pod is required")
	}
	if pod.Namespace == "" || pod.Name == "" {
		return nil, fmt.Errorf("pod namespace and name are required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}

	err := c.Create(ctx, pod)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastPhase corev1.PodPhase

	for {
		current := &corev1.Pod{}
		if err := c.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, current); err != nil {
			return nil, fmt.Errorf("failed to get pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}

		lastPhase = current.Status.Phase
		switch current.Status.Phase {
		case corev1.PodSucceeded, corev1.PodFailed:
			logs, logsErr := getPodLogs(ctx, cfg, current.Namespace, current.Name)
			if logsErr != nil {
				return nil, fmt.Errorf("failed to get pod logs for %s/%s: %w", current.Namespace, current.Name, logsErr)
			}

			return &PodResult{
				Namespace: current.Namespace,
				Name:      current.Name,
				Phase:     current.Status.Phase,
				Logs:      logs,
			}, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf(
				"context canceled while waiting for pod %s/%s to complete: %w",
				pod.Namespace,
				pod.Name,
				ctx.Err(),
			)
		case <-deadline.C:
			return nil, fmt.Errorf(
				"timed out waiting for pod %s/%s to complete (last phase: %s)",
				pod.Namespace,
				pod.Name,
				lastPhase,
			)
		case <-ticker.C:
		}
	}
}

func getPodLogs(ctx context.Context, cfg *rest.Config, namespace string, name string) (string, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %w", err)
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{})
	raw, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// DeletePodBestEffort deletes a pod and ignores NotFound.
func DeletePodBestEffort(ctx context.Context, c client.Client, namespace string, name string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}

	err := c.Delete(ctx, pod)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
