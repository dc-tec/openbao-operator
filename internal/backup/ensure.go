package backup

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const ssaFieldOwner = "openbao-operator"

// EnsureBackupServiceAccount creates or updates the ServiceAccount for backup/snapshot Jobs.
func EnsureBackupServiceAccount(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := backupServiceAccountName(cluster)
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: cluster.Namespace,
			Labels:    backupLabels(cluster),
		},
	}

	if err := applyResource(ctx, c, scheme, sa, cluster, ssaFieldOwner); err != nil {
		return fmt.Errorf("failed to ensure backup ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// EnsureBackupRBAC creates or updates the Role/RoleBinding required by backup Jobs for pod discovery.
func EnsureBackupRBAC(ctx context.Context, c client.Client, scheme *runtime.Scheme, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := backupServiceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"
	resourceLabels := backupLabels(cluster)

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: cluster.Namespace,
			Labels:    resourceLabels,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	if err := applyResource(ctx, c, scheme, role, cluster, ssaFieldOwner); err != nil {
		return fmt.Errorf("failed to ensure Role %s/%s: %w", cluster.Namespace, roleName, err)
	}

	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: cluster.Namespace,
			Labels:    resourceLabels,
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

	if err := applyResource(ctx, c, scheme, roleBinding, cluster, ssaFieldOwner); err != nil {
		return fmt.Errorf("failed to ensure RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
	}

	return nil
}
