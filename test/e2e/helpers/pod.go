package helpers

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	podNotFoundGracePeriod = 10 * time.Second
	maxPodsInSummary       = 10
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

	waitStart := time.Now()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastPhase corev1.PodPhase

	for {
		current := &corev1.Pod{}
		if err := c.Get(ctx, types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, current); err != nil {
			if apierrors.IsNotFound(err) && time.Since(waitStart) < podNotFoundGracePeriod {
				// The API server may not observe the object immediately, especially under load.
				// Treat NotFound as transient for a short grace period to reduce flakes.
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf(
						"context canceled while waiting for pod %s/%s to appear: %w",
						pod.Namespace,
						pod.Name,
						ctx.Err(),
					)
				case <-deadline.C:
					return nil, fmt.Errorf("timed out waiting for pod %s/%s to appear", pod.Namespace, pod.Name)
				case <-ticker.C:
					continue
				}
			}

			if apierrors.IsNotFound(err) {
				podsSummary, summaryErr := summarizePods(ctx, c, pod.Namespace)
				if summaryErr != nil {
					return nil, fmt.Errorf(
						"pod %s/%s disappeared while waiting (failed to list pods for diagnostics: %v)",
						pod.Namespace,
						pod.Name,
						summaryErr,
					)
				}
				return nil, fmt.Errorf(
					"pod %s/%s disappeared while waiting; pods in namespace:\n%s",
					pod.Namespace,
					pod.Name,
					podsSummary,
				)
			}

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

func summarizePods(ctx context.Context, c client.Client, namespace string) (string, error) {
	var pods corev1.PodList
	if err := c.List(ctx, &pods, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "(no pods found)", nil
	}

	lines := make([]string, 0, minInt(len(pods.Items), maxPodsInSummary))
	for i := range pods.Items {
		if len(lines) >= maxPodsInSummary {
			break
		}
		pod := pods.Items[i]
		lines = append(lines, fmt.Sprintf("- %s phase=%s", pod.Name, pod.Status.Phase))
	}
	if len(pods.Items) > maxPodsInSummary {
		lines = append(lines, fmt.Sprintf("... and %d more", len(pods.Items)-maxPodsInSummary))
	}
	return strings.Join(lines, "\n"), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
