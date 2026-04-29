//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/test/e2e/framework"
)

const (
	controllerDeploymentName = "openbao-operator-controller"
	controllerLeaderLease    = "openbao-controller-leader.openbao.org"
)

func getControllerDeployment(ctx context.Context, c client.Client, namespace string) (*appsv1.Deployment, error) {
	deploy := &appsv1.Deployment{}
	key := types.NamespacedName{Name: controllerDeploymentName, Namespace: namespace}
	if err := c.Get(ctx, key, deploy); err != nil {
		return nil, fmt.Errorf("get controller deployment %s/%s: %w", namespace, controllerDeploymentName, err)
	}
	return deploy, nil
}

func scaleControllerDeployment(ctx context.Context, c client.Client, namespace string, replicas int32) error {
	deploy, err := getControllerDeployment(ctx, c, namespace)
	if err != nil {
		return err
	}

	original := deploy.DeepCopy()
	deploy.Spec.Replicas = &replicas
	if err := c.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("patch controller deployment replicas: %w", err)
	}

	return waitForControllerDeploymentReplicas(ctx, c, namespace, replicas, 5*time.Minute, framework.DefaultPollInterval)
}

func waitForControllerDeploymentReplicas(
	ctx context.Context,
	c client.Client,
	namespace string,
	replicas int32,
	timeout time.Duration,
	pollInterval time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deploy, err := getControllerDeployment(ctx, c, namespace)
		if err != nil {
			return err
		}

		if deploy.Generation <= deploy.Status.ObservedGeneration {
			if replicas == 0 {
				pods, err := listControllerPods(ctx, c, namespace)
				if err != nil {
					return err
				}
				if len(pods) == 0 {
					return nil
				}
			} else if deploy.Status.ReadyReplicas == replicas && deploy.Status.UpdatedReplicas == replicas {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for controller replicas=%d: %w", replicas, ctx.Err())
		case <-time.After(pollInterval):
		}
	}

	return fmt.Errorf("timed out waiting for controller deployment replicas=%d", replicas)
}

func listControllerPods(ctx context.Context, c client.Client, namespace string) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := c.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":      "openbao-operator",
			"app.kubernetes.io/component": "controller",
		},
	); err != nil {
		return nil, fmt.Errorf("list controller pods: %w", err)
	}

	ready := make([]corev1.Pod, 0, len(pods.Items))
	for i := range pods.Items {
		pod := pods.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		ready = append(ready, pod)
	}
	return ready, nil
}

func waitForReadyControllerPods(
	ctx context.Context,
	c client.Client,
	namespace string,
	count int,
	timeout time.Duration,
) ([]corev1.Pod, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pods, err := listControllerPods(ctx, c, namespace)
		if err != nil {
			return nil, err
		}

		ready := make([]corev1.Pod, 0, len(pods))
		for i := range pods {
			if isPodReady(&pods[i]) {
				ready = append(ready, pods[i])
			}
		}
		if len(ready) == count {
			return ready, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled while waiting for %d ready controller pods: %w", count, ctx.Err())
		case <-time.After(framework.DefaultPollInterval):
		}
	}

	return nil, fmt.Errorf("timed out waiting for %d ready controller pods", count)
}

func controllerLeaderHolderIdentity(ctx context.Context, c client.Client, namespace string) (string, error) {
	lease := &coordinationv1.Lease{}
	key := types.NamespacedName{Name: controllerLeaderLease, Namespace: namespace}
	if err := c.Get(ctx, key, lease); err != nil {
		return "", fmt.Errorf("get controller leader lease %s/%s: %w", namespace, controllerLeaderLease, err)
	}
	if lease.Spec.HolderIdentity == nil {
		return "", fmt.Errorf("controller leader lease %s/%s has no holder identity", namespace, controllerLeaderLease)
	}
	return *lease.Spec.HolderIdentity, nil
}

func deleteControllerPod(ctx context.Context, c client.Client, namespace, name string) error {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	if err := c.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete controller pod %s/%s: %w", namespace, name, err)
	}
	return nil
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
