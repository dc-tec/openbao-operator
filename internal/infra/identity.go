package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
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
			Name:        saName,
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: nil,
		},
	}

	if cluster.Spec.ServiceAccount != nil && cluster.Spec.ServiceAccount.Annotations != nil {
		sa.Annotations = cluster.Spec.ServiceAccount.Annotations
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
	if cluster.Spec.ServiceAccount != nil && cluster.Spec.ServiceAccount.Name != "" {
		return cluster.Spec.ServiceAccount.Name
	}
	return cluster.Name + constants.SuffixServiceAccount
}
