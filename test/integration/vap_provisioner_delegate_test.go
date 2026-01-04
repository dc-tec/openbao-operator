//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const provisionerDelegateUsername = "system:serviceaccount:openbao-operator-system:openbao-operator-provisioner-delegate"

func TestVAP_ProvisionerDelegate_DeniesWrongRoleName(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerDelegateRBACApplied(t)

	namespace := newTestNamespace(t)
	delegateClient := newImpersonatedClient(t, provisionerDelegateUsername)

	// Admission policies can take a short moment to become effective after apply.
	// Retry until the request is denied, failing if it never happens.
	for attempt := 0; attempt < 25; attempt++ {
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "evil-role",
				Namespace: namespace,
			},
			Rules: []rbacv1.PolicyRule{},
		}

		err := delegateClient.Create(ctx, role)
		if err == nil {
			_ = k8sClient.Delete(ctx, role)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "Provisioner Delegate can only create Roles") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	// Sanity: if the Role still exists, clean it up so later tests don't interact with it.
	var existing rbacv1.Role
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "evil-role"}, &existing); err == nil {
		_ = k8sClient.Delete(ctx, &existing)
	} else if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("get evil-role after retries: %v", err)
	}

	t.Fatalf("expected VAP to deny creating Role with non-allowed name after retries")
}

func TestVAP_ProvisionerDelegate_RestrictsRoleBindingSubjects(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerDelegateRBACApplied(t)

	namespace := newTestNamespace(t)
	delegateClient := newImpersonatedClient(t, provisionerDelegateUsername)

	// Some API servers validate RoleBinding.roleRef existence; create the Role first.
	tenantRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao-operator-tenant-role",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{},
	}
	if err := delegateClient.Create(ctx, tenantRole); err != nil {
		t.Fatalf("create tenant Role: %v", err)
	}

	tenantRB := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao-operator-tenant-rolebinding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "openbao-operator-tenant-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-operator-controller",
				Namespace: "openbao-operator-system",
			},
		},
	}

	if err := delegateClient.Create(ctx, tenantRB); err != nil {
		t.Fatalf("expected tenant RoleBinding creation to succeed, got: %v", err)
	}

	// Attempt to broaden subject namespace; should be denied by the VAP.
	var latest rbacv1.RoleBinding
	if err := delegateClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: tenantRB.Name}, &latest); err != nil {
		t.Fatalf("get RoleBinding: %v", err)
	}
	latest.Subjects[0].Namespace = "kube-system"
	err := delegateClient.Update(ctx, &latest)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "can only bind tenant RBAC") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_ProvisionerDelegate_RestrictsSentinelRoleBindingSubjectNamespace(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	ensureProvisionerDelegateRBACApplied(t)

	namespace := newTestNamespace(t)
	delegateClient := newImpersonatedClient(t, provisionerDelegateUsername)

	sentinelRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao-sentinel-role",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{},
	}
	if err := delegateClient.Create(ctx, sentinelRole); err != nil {
		t.Fatalf("create sentinel Role: %v", err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openbao-sentinel-rolebinding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "openbao-sentinel-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-sentinel",
				Namespace: namespace,
			},
		},
	}

	if err := delegateClient.Create(ctx, rb); err != nil {
		t.Fatalf("expected sentinel RoleBinding creation to succeed, got: %v", err)
	}

	var latest rbacv1.RoleBinding
	if err := delegateClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: rb.Name}, &latest); err != nil {
		t.Fatalf("get RoleBinding: %v", err)
	}
	latest.Subjects[0].Namespace = "openbao-operator-system"
	err := delegateClient.Update(ctx, &latest)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "can only bind tenant RBAC") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
