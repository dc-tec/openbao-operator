//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	provisionerpkg "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

func newPrivilegedImpersonatedClient(t *testing.T, username string) client.Client {
	t.Helper()

	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   []string{"system:masters"},
	}

	c, err := client.New(impersonated, client.Options{Scheme: k8sScheme})
	if err != nil {
		t.Fatalf("create privileged impersonated client: %v", err)
	}
	return c
}

func ensureControllerRBACManager(t *testing.T, namespace string) {
	t.Helper()

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "controller-rbac-manager",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"roles", "rolebindings"},
				Verbs:     []string{"create", "delete", "get", "patch", "update"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create controller rbac role: %v", err)
	}

	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "controller-rbac-manager-binding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-operator-controller",
				Namespace: "openbao-operator-system",
			},
		},
	}
	if err := k8sClient.Create(ctx, binding); err != nil {
		t.Fatalf("create controller rbac rolebinding: %v", err)
	}
}

func TestVAP_LockManagedRBAC_DeniesDirectMutationOfControllerManagedRole(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	ensureControllerRBACManager(t, namespace)
	controllerClient := newPrivilegedImpersonatedClient(t, controllerUsername)
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-serviceaccount-role",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   "example",
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          "example",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"pods"},
				ResourceNames: []string{"example-0", "example-1", "example-2"},
				Verbs:         []string{"patch", "update"},
			},
		},
	}
	if err := controllerClient.Create(ctx, role); err != nil {
		t.Fatalf("create managed Role: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest rbacv1.Role
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: role.Name}, &latest); err != nil {
			t.Fatalf("get managed Role: %v", err)
		}

		original := latest.DeepCopy()
		latest.Rules = append(latest.Rules, rbacv1.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get"},
		})
		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "Direct modification of OpenBao-managed resources is prohibited") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny direct mutation of controller-managed Role after retries")
}

func TestVAP_LockManagedRBAC_DeniesDirectMutationOfProvisionerManagedRoleBinding(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerRBACApplied(t)

	namespace := newTestNamespace(t)
	provisionerClient := newPrivilegedImpersonatedClient(t, provisionerUsername)
	tenantRole := provisionerpkg.GenerateTenantRole(namespace)
	if err := provisionerClient.Create(ctx, tenantRole); err != nil {
		t.Fatalf("create tenant Role: %v", err)
	}

	roleBinding := provisionerpkg.GenerateTenantRoleBinding(namespace, provisionerpkg.OperatorServiceAccount{
		Name:      "openbao-operator-controller",
		Namespace: "openbao-operator-system",
	})
	if err := provisionerClient.Create(ctx, roleBinding); err != nil {
		t.Fatalf("create managed RoleBinding: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest rbacv1.RoleBinding
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: roleBinding.Name}, &latest); err != nil {
			t.Fatalf("get managed RoleBinding: %v", err)
		}

		original := latest.DeepCopy()
		latest.Subjects = append(latest.Subjects, rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      "unexpected",
			Namespace: namespace,
		})
		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "Direct modification of OpenBao-managed resources is prohibited") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny direct mutation of provisioner-managed RoleBinding after retries")
}
