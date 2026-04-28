//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	provisionerpkg "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

func newPrivilegedImpersonatedClient(t *testing.T, username string) client.Client {
	t.Helper()

	return newImpersonatedClientWithGroups(t, username, "system:masters")
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

func grantPodDeleteAccess(t *testing.T, namespace, username string) {
	t.Helper()

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-pod-maintenance",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"delete", "get"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create pod maintenance role: %v", err)
	}

	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-pod-maintenance-binding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     username,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
	}
	if err := k8sClient.Create(ctx, binding); err != nil {
		t.Fatalf("create pod maintenance binding: %v", err)
	}
}

func grantClusterMaintenanceAccess(t *testing.T, namespace, clusterName, username string) {
	t.Helper()

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-maintenance-access",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"openbao.org"},
				Resources:     []string{"openbaoclusters"},
				ResourceNames: []string{clusterName},
				Verbs:         []string{"get", "maintenance"},
			},
			{
				APIGroups:     []string{"openbao.org"},
				Resources:     []string{"openbaoclusters/status"},
				ResourceNames: []string{clusterName},
				Verbs:         []string{"get"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create maintenance role: %v", err)
	}

	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-maintenance-access-binding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     username,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
	}
	if err := k8sClient.Create(ctx, binding); err != nil {
		t.Fatalf("create maintenance rolebinding: %v", err)
	}
}

func createManagedMaintenancePod(t *testing.T, c client.Client, namespace, clusterName, podName string) {
	t.Helper()

	cluster := newMinimalClusterObj(namespace, clusterName)
	cluster.Spec.InitContainer = &openbaov1alpha1.InitContainerConfig{
		Image: "openbao/openbao-init:latest",
	}
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeOperatorManaged
	if err := c.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Annotations: map[string]string{
				"openbao.org/maintenance": testTrueString,
			},
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   clusterName,
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          clusterName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "pause",
					Image: "registry.k8s.io/pause:3.9",
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	if err := c.Create(ctx, pod); err != nil {
		t.Fatalf("create managed maintenance pod: %v", err)
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

func TestVAP_LockManagedRBAC_DeniesMaintenanceMutationWithoutClusterMaintenanceVerb(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	clusterName := "example"
	podName := "example-maintenance-pod"
	editorUsername := "maintenance-editor-no-verb"
	controllerClient := newPrivilegedImpersonatedClient(t, controllerUsername)
	createManagedMaintenancePod(t, controllerClient, namespace, clusterName, podName)
	grantPodDeleteAccess(t, namespace, editorUsername)

	editorClient := newImpersonatedClient(t, editorUsername)
	for attempt := 0; attempt < 25; attempt++ {
		err := editorClient.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace}})
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

	t.Fatalf("expected VAP to deny maintenance mutation without the cluster maintenance verb")
}

func TestVAP_LockManagedRBAC_AllowsMaintenanceMutationWithClusterMaintenanceVerb(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	clusterName := "example"
	podName := "example-maintenance-pod"
	editorUsername := "maintenance-editor"
	controllerClient := newPrivilegedImpersonatedClient(t, controllerUsername)
	createManagedMaintenancePod(t, controllerClient, namespace, clusterName, podName)
	grantPodDeleteAccess(t, namespace, editorUsername)
	grantClusterMaintenanceAccess(t, namespace, clusterName, editorUsername)

	editorClient := newImpersonatedClient(t, editorUsername)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: namespace}}
	if err := editorClient.Delete(ctx, pod); err != nil {
		t.Fatalf("expected maintenance-authorized pod delete to succeed, got: %v", err)
	}

	var deleted corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, &deleted); err == nil {
		t.Fatalf("expected Pod %s/%s to be deleted", namespace, podName)
	}
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
		roleBindingKey := types.NamespacedName{Namespace: namespace, Name: roleBinding.Name}
		if err := k8sClient.Get(ctx, roleBindingKey, &latest); err != nil {
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
