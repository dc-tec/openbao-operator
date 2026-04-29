//go:build integration
// +build integration

package integration

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ensureControllerResourceRBAC(
	t *testing.T,
	namespace string,
	roleName string,
	bindingName string,
	resource string,
) {
	t.Helper()

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: testRBACV1,
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{resource},
				Verbs:     []string{"create", "delete", "get", "patch", "update"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create controller %s role: %v", resource, err)
	}

	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: testRBACV1,
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: testRBACGroup,
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      testControllerSAName,
				Namespace: testDefaultOperatorNS,
			},
		},
	}
	if err := k8sClient.Create(ctx, binding); err != nil {
		t.Fatalf("create controller %s rolebinding: %v", resource, err)
	}
}
