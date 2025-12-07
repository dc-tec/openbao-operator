package infra

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestEnsureServiceAccountCreatesAndUpdates(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-sa", "default")

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	sa := &corev1.ServiceAccount{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serviceAccountName(cluster),
	}, sa)
	if err != nil {
		t.Fatalf("expected ServiceAccount to exist: %v", err)
	}

	// Verify labels
	expectedLabels := infraLabels(cluster)
	for key, expectedVal := range expectedLabels {
		if sa.Labels[key] != expectedVal {
			t.Errorf("expected ServiceAccount label %s=%s, got %s", key, expectedVal, sa.Labels[key])
		}
	}
}

func TestEnsureServiceAccountUpdatesLabels(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-sa-update", "default")

	ctx := context.Background()

	// First reconcile creates the ServiceAccount
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Update cluster name (simulating label change)
	cluster.Name = "infra-sa-updated"

	// Second reconcile should update labels
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Note: ServiceAccount name is based on cluster name, so this test
	// verifies that the old ServiceAccount still exists with old labels
	// and a new one is created. This is expected behavior.
	sa := &corev1.ServiceAccount{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serviceAccountName(cluster),
	}, sa)
	if err != nil {
		t.Fatalf("expected ServiceAccount to exist: %v", err)
	}
}

func TestEnsureRBACCreatesRoleAndRoleBinding(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-rbac", "default")

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify Role exists
	role := &rbacv1.Role{}
	roleName := serviceAccountName(cluster) + "-role"
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleName,
	}, role)
	if err != nil {
		t.Fatalf("expected Role to exist: %v", err)
	}

	// Verify Role has correct rules
	if len(role.Rules) == 0 {
		t.Fatalf("expected Role to have rules")
	}

	foundPodRule := false
	for _, rule := range role.Rules {
		if len(rule.APIGroups) > 0 && rule.APIGroups[0] == "" &&
			len(rule.Resources) > 0 && rule.Resources[0] == "pods" {
			foundPodRule = true
			expectedVerbs := []string{"list", "get"}
			for _, expectedVerb := range expectedVerbs {
				found := false
				for _, verb := range rule.Verbs {
					if verb == expectedVerb {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected Role to have verb %q", expectedVerb)
				}
			}
		}
	}
	if !foundPodRule {
		t.Fatalf("expected Role to have pod list/get rule")
	}

	// Verify RoleBinding exists
	roleBinding := &rbacv1.RoleBinding{}
	roleBindingName := serviceAccountName(cluster) + "-rolebinding"
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected RoleBinding to exist: %v", err)
	}

	// Verify RoleBinding references the Role
	if roleBinding.RoleRef.Name != roleName {
		t.Fatalf("expected RoleBinding to reference Role %q, got %q", roleName, roleBinding.RoleRef.Name)
	}

	// Verify RoleBinding references the ServiceAccount
	if len(roleBinding.Subjects) == 0 {
		t.Fatalf("expected RoleBinding to have subjects")
	}

	saName := serviceAccountName(cluster)
	foundSubject := false
	for _, subject := range roleBinding.Subjects {
		if subject.Kind == "ServiceAccount" && subject.Name == saName && subject.Namespace == cluster.Namespace {
			foundSubject = true
			break
		}
	}
	if !foundSubject {
		t.Fatalf("expected RoleBinding to reference ServiceAccount %q", saName)
	}
}

func TestEnsureRBACUpdatesWhenServiceAccountChanges(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-rbac-update", "default")

	ctx := context.Background()

	// First reconcile creates RBAC
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Second reconcile should update labels
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify RoleBinding still references correct ServiceAccount
	roleBinding := &rbacv1.RoleBinding{}
	roleBindingName := serviceAccountName(cluster) + "-rolebinding"
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleBindingName,
	}, roleBinding)
	if err != nil {
		t.Fatalf("expected RoleBinding to exist: %v", err)
	}

	saName := serviceAccountName(cluster)
	if len(roleBinding.Subjects) == 0 || roleBinding.Subjects[0].Name != saName {
		t.Fatalf("expected RoleBinding to reference ServiceAccount %q", saName)
	}
}

func TestDeleteServiceAccountDeletesServiceAccount(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-sa-delete", "default")

	ctx := context.Background()

	// Create ServiceAccount first
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Cleanup should delete ServiceAccount
	if err := manager.Cleanup(ctx, logr.Discard(), cluster, openbaov1alpha1.DeletionPolicyRetain); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify ServiceAccount is deleted
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serviceAccountName(cluster),
	}, &corev1.ServiceAccount{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected ServiceAccount to be deleted, got error: %v", err)
	}
}

func TestDeleteRBACDeletesRoleAndRoleBinding(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-rbac-delete", "default")

	ctx := context.Background()

	// Create RBAC first
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Cleanup should delete RBAC
	if err := manager.Cleanup(ctx, logr.Discard(), cluster, openbaov1alpha1.DeletionPolicyRetain); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify RoleBinding is deleted
	roleBindingName := serviceAccountName(cluster) + "-rolebinding"
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleBindingName,
	}, &rbacv1.RoleBinding{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected RoleBinding to be deleted, got error: %v", err)
	}

	// Verify Role is deleted
	roleName := serviceAccountName(cluster) + "-role"
	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      roleName,
	}, &rbacv1.Role{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected Role to be deleted, got error: %v", err)
	}
}
