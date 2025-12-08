package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

// ensureServiceAccount manages the ServiceAccount for the OpenBaoCluster.
func (m *Manager) ensureServiceAccount(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)

	sa := &corev1.ServiceAccount{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      saName,
	}, sa)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}

		logger.Info("ServiceAccount not found; creating", "serviceaccount", saName)

		sa = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: cluster.Namespace,
				Labels:    infraLabels(cluster),
			},
		}

		// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
		if err := controllerutil.SetControllerReference(cluster, sa, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}

		if err := m.client.Create(ctx, sa); err != nil {
			return fmt.Errorf("failed to create ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
		}

		return nil
	}

	// Update labels if needed
	if sa.Labels == nil {
		sa.Labels = make(map[string]string)
	}
	for k, v := range infraLabels(cluster) {
		sa.Labels[k] = v
	}

	if err := m.client.Update(ctx, sa); err != nil {
		return fmt.Errorf("failed to update ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// ensureRBAC creates a Role and RoleBinding that grants the OpenBaoCluster service account
// permission to list pods in its namespace. This is required for OpenBao's Kubernetes
// auto-join discovery mechanism to work.
func (m *Manager) ensureRBAC(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"

	// Ensure Role exists
	role := &rbacv1.Role{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleName,
	}, role)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Role %s/%s: %w", cluster.Namespace, roleName, err)
		}

		logger.Info("Role not found; creating", "role", roleName)

		role = &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: cluster.Namespace,
				Labels:    infraLabels(cluster),
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"list", "get"},
				},
			},
		}

		if err := controllerutil.SetControllerReference(cluster, role, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on Role %s/%s: %w", cluster.Namespace, roleName, err)
		}

		if err := m.client.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create Role %s/%s: %w", cluster.Namespace, roleName, err)
		}
	} else {
		// Update labels if needed
		if role.Labels == nil {
			role.Labels = make(map[string]string)
		}
		for k, v := range infraLabels(cluster) {
			role.Labels[k] = v
		}

		if err := m.client.Update(ctx, role); err != nil {
			return fmt.Errorf("failed to update Role %s/%s: %w", cluster.Namespace, roleName, err)
		}
	}

	// Ensure RoleBinding exists
	roleBinding := &rbacv1.RoleBinding{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleBindingName,
	}, roleBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
		}

		logger.Info("RoleBinding not found; creating", "rolebinding", roleBindingName)

		roleBinding = &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleBindingName,
				Namespace: cluster.Namespace,
				Labels:    infraLabels(cluster),
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     roleName,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: cluster.Namespace,
				},
			},
		}

		if err := controllerutil.SetControllerReference(cluster, roleBinding, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
		}

		if err := m.client.Create(ctx, roleBinding); err != nil {
			return fmt.Errorf("failed to create RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
		}
	} else {
		// Update labels if needed
		if roleBinding.Labels == nil {
			roleBinding.Labels = make(map[string]string)
		}
		for k, v := range infraLabels(cluster) {
			roleBinding.Labels[k] = v
		}

		// Ensure RoleRef and Subjects are correct
		needsUpdate := false
		if roleBinding.RoleRef.Name != roleName {
			roleBinding.RoleRef.Name = roleName
			needsUpdate = true
		}
		if len(roleBinding.Subjects) != 1 || roleBinding.Subjects[0].Name != saName {
			roleBinding.Subjects = []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: cluster.Namespace,
				},
			}
			needsUpdate = true
		}

		if needsUpdate {
			if err := m.client.Update(ctx, roleBinding); err != nil {
				return fmt.Errorf("failed to update RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
			}
		} else if len(roleBinding.Labels) > 0 {
			// Only update if labels changed
			if err := m.client.Update(ctx, roleBinding); err != nil {
				return fmt.Errorf("failed to update RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
			}
		}
	}

	return nil
}

// serviceAccountName returns the name for the ServiceAccount resource.
func serviceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + serviceAccountSuffix
}

// deleteServiceAccount removes the ServiceAccount for the OpenBaoCluster.
func (m *Manager) deleteServiceAccount(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	sa := &corev1.ServiceAccount{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serviceAccountName(cluster),
	}, sa)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := m.client.Delete(ctx, sa); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// deleteRBAC removes the Role and RoleBinding for the OpenBaoCluster.
func (m *Manager) deleteRBAC(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"

	// Delete RoleBinding first
	roleBinding := &rbacv1.RoleBinding{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleBindingName,
	}, roleBinding)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
		}
	} else {
		if err := m.client.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
		}
	}

	// Delete Role
	role := &rbacv1.Role{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleName,
	}, role)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get Role %s/%s: %w", cluster.Namespace, roleName, err)
		}
	} else {
		if err := m.client.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete Role %s/%s: %w", cluster.Namespace, roleName, err)
		}
	}

	return nil
}
