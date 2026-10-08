package restore

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kubebao/openbao-operator/api/v1alpha1"
)

// restoreExecutorPending vetoes a terminal Job observation when its remaining
// Pods do not report terminated containers. It does not prove node fencing or
// server-side snapshot application. In particular, missing Pods are not process
// termination evidence for administrator recovery.
func (m *Manager) restoreExecutorPending(ctx context.Context, job *batchv1.Job) (string, error) {
	if job.UID == "" {
		return "", fmt.Errorf("restore Job UID is required for executor observation")
	}
	// Inspect ownership across the namespace: executor labels can be missing or
	// stale, and a same-name replacement Job must not inherit another Job's Pods.
	options := &client.ListOptions{Namespace: job.Namespace, Limit: 500}
	for {
		pods := &corev1.PodList{}
		if err := m.reader.List(ctx, pods, options); err != nil {
			return "", fmt.Errorf("list restore executor Pods: %w", err)
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			owner := metav1.GetControllerOf(pod)
			if owner == nil || owner.UID != job.UID {
				continue
			}
			if owner.APIVersion != batchv1.SchemeGroupVersion.String() || owner.Kind != "Job" || owner.Name != job.Name {
				return "", fmt.Errorf("restore executor Pod %s has inconsistent Job ownership", pod.Name)
			}
			if !restoreExecutorContainersTerminated(pod) {
				return fmt.Sprintf("Waiting for restore executor Pod %s to report termination of all containers.", pod.Name), nil
			}
		}
		if pods.Continue == "" {
			return "", nil
		}
		options.Continue = pods.Continue
	}
}

func restoreExecutorContainersTerminated(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
		return false
	}
	if len(pod.Spec.Containers) == 0 {
		return false
	}
	terminated := func(name string, statuses []corev1.ContainerStatus) bool {
		for _, status := range statuses {
			if status.Name == name {
				return status.State.Terminated != nil
			}
		}
		return false
	}
	for _, container := range pod.Spec.Containers {
		if !terminated(container.Name, pod.Status.ContainerStatuses) {
			return false
		}
	}
	for _, container := range pod.Spec.InitContainers {
		if !terminated(container.Name, pod.Status.InitContainerStatuses) {
			return false
		}
	}
	for _, container := range pod.Spec.EphemeralContainers {
		if !terminated(container.Name, pod.Status.EphemeralContainerStatuses) {
			return false
		}
	}
	return true
}

// Admission binds the token to a Pod UID; the controller checks the independent
// Job ownership boundary before accepting that Pod as the recorded executor.
// Labels and owner references assume trusted destination Pod writers. They
// correlate the claimant with the Job; they do not authenticate a tenant's Pod.
func (m *Manager) validateClaimExecutor(ctx context.Context, request *api.OpenBaoRestore) (string, error) {
	if request.Status.SubmissionClaim == nil {
		return "", nil
	}

	execution := request.Status.Execution
	if execution == nil || execution.JobUID == "" {
		return "submission claimant has no recorded restore Job", nil
	}

	options := &client.ListOptions{
		Namespace: request.Namespace, Limit: 500,
		LabelSelector: labels.SelectorFromSet(labels.Set{batchv1.ControllerUidLabel: string(execution.JobUID)}),
	}
	for {
		pods := &corev1.PodList{}
		if err := m.reader.List(ctx, pods, options); err != nil {
			return "", fmt.Errorf("read submission claimant: %w", err)
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.UID != request.Status.SubmissionClaim.PodUID {
				continue
			}
			owner := metav1.GetControllerOf(pod)
			if owner == nil || owner.APIVersion != "batch/v1" || owner.Kind != "Job" ||
				owner.UID != execution.JobUID || owner.Name != execution.JobName {
				return "submission claimant does not belong to the recorded restore Job", nil
			}
			return "", nil
		}
		if pods.Continue == "" {
			return "submission claimant Pod is absent; its Job ownership cannot be checked", nil
		}
		options.Continue = pods.Continue
	}
}
