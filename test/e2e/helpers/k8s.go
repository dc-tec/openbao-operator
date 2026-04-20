package helpers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RunWithImpersonation runs the provided action as the specified user/group(s)
// using Kubernetes impersonation. This is used in E2E tests to validate RBAC and
// ValidatingAdmissionPolicy enforcement under explicit caller identities.
func RunWithImpersonation(
	ctx context.Context,
	baseConfig *rest.Config,
	scheme *runtime.Scheme,
	username string,
	groups []string,
	action func(c client.Client) error,
) error {
	if baseConfig == nil {
		return fmt.Errorf("base Kubernetes REST config is required")
	}
	if scheme == nil {
		return fmt.Errorf("runtime Scheme is required")
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if action == nil {
		return fmt.Errorf("action is required")
	}

	impersonatedConfig := rest.CopyConfig(baseConfig)
	impersonatedConfig.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   groups,
	}

	impersonatedClient, err := client.New(impersonatedConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create impersonated client for user %q: %w", username, err)
	}

	if err := action(impersonatedClient); err != nil {
		return err
	}

	return nil
}

// EnsureRoleBinding creates a Role and same-named RoleBinding if they do not already exist.
func EnsureRoleBinding(ctx context.Context, c client.Client, role *rbacv1.Role, subjects []rbacv1.Subject) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if c == nil {
		return fmt.Errorf("kubernetes client is required")
	}
	if role == nil {
		return fmt.Errorf("role is required")
	}
	if role.Name == "" || role.Namespace == "" {
		return fmt.Errorf("role name and namespace are required")
	}
	if len(subjects) == 0 {
		return fmt.Errorf("at least one subject is required")
	}

	if err := c.Create(ctx, role); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create role %s/%s: %w", role.Namespace, role.Name, err)
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      role.Name + "-binding",
			Namespace: role.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: subjects,
	}
	if err := c.Create(ctx, binding); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create rolebinding %s/%s: %w", binding.Namespace, binding.Name, err)
	}

	return nil
}

// waitForDeploymentReady waits for a Deployment to become ready with all replicas available.
// It checks pod status for debugging and provides detailed error messages.
func waitForDeploymentReady(
	ctx context.Context,
	c client.Client,
	namespace, name, componentName string,
) error {
	deploymentReadyDeadline := time.NewTimer(5 * time.Minute)
	defer deploymentReadyDeadline.Stop()
	deploymentReadyTicker := time.NewTicker(2 * time.Second)
	defer deploymentReadyTicker.Stop()

	for {
		current := &appsv1.Deployment{}
		if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, current); err != nil {
			return fmt.Errorf("failed to get %s Deployment %s/%s: %w", componentName, namespace, name, err)
		}

		if current.Status.ReadyReplicas >= 1 && current.Status.ReadyReplicas == current.Status.Replicas {
			break
		}

		// Check pod status for debugging
		var pods corev1.PodList
		if err := c.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{"app": name}); err == nil {
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodFailed {
					return fmt.Errorf(
						"%s pod %s/%s failed: %v",
						componentName,
						pod.Namespace,
						pod.Name,
						pod.Status.ContainerStatuses,
					)
				}
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf(
				"context canceled while waiting for %s Deployment %s/%s to be ready: %w",
				componentName,
				namespace,
				name,
				ctx.Err(),
			)
		case <-deploymentReadyDeadline.C:
			return fmt.Errorf(
				"timed out waiting for %s Deployment %s/%s to be ready (ready=%d/%d, replicas=%d)",
				componentName,
				namespace,
				name,
				current.Status.ReadyReplicas,
				1,
				current.Status.Replicas,
			)
		case <-deploymentReadyTicker.C:
		}
	}

	return nil
}
