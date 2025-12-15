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

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
)

// ensureServiceAccount manages the ServiceAccount for the OpenBaoCluster using Server-Side Apply.
func (m *Manager) ensureServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
	}

	if err := m.applyResource(ctx, sa, cluster); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// ensureRBAC creates a Role and RoleBinding that grants the OpenBaoCluster service account
// permission to list pods in its namespace. This is required for OpenBao's Kubernetes
// auto-join discovery mechanism to work. Uses Server-Side Apply.
func (m *Manager) ensureRBAC(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"

	// Ensure Role exists using SSA
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch", "patch", "update"},
			},
		},
	}

	if err := m.applyResource(ctx, role, cluster); err != nil {
		return fmt.Errorf("failed to ensure Role %s/%s: %w", cluster.Namespace, roleName, err)
	}

	// Ensure RoleBinding exists using SSA
	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
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

	if err := m.applyResource(ctx, roleBinding, cluster); err != nil {
		return fmt.Errorf("failed to ensure RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
	}

	return nil
}

// serviceAccountName returns the name for the ServiceAccount resource.
func serviceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixServiceAccount
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
