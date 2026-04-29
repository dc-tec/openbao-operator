//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func ensureControllerServiceAccountRBAC(t *testing.T, namespace string) {
	t.Helper()

	ensureControllerResourceRBAC(
		t,
		namespace,
		"controller-serviceaccount-manager",
		"controller-serviceaccount-manager-binding",
		"serviceaccounts",
	)
}

func TestVAP_ControllerServiceAccounts_DeniesUnexpectedServiceAccount(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	ensureControllerServiceAccountRBAC(t, namespace)
	controllerClient := newImpersonatedClient(t, controllerUsername)

	for attempt := 0; attempt < 25; attempt++ {
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-serviceaccount",
				Namespace: namespace,
			},
		}

		err := controllerClient.Create(ctx, sa)
		if err == nil {
			_ = k8sClient.Delete(ctx, sa)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "controller can only create, update, or delete operator-managed ServiceAccounts") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny controller ServiceAccount creation with unexpected labels/name after retries")
}

func TestVAP_ControllerServiceAccounts_AllowsManagedBackupServiceAccount(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	ensureControllerServiceAccountRBAC(t, namespace)
	controllerClient := newImpersonatedClient(t, controllerUsername)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-backup-serviceaccount",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":           "openbao",
				"app.kubernetes.io/instance":       "example",
				"app.kubernetes.io/managed-by":     "openbao-operator",
				"openbao.org/cluster":              "example",
				"openbao.org/component":            "backup",
				"openbao.org/service-account-role": "backup",
			},
		},
	}

	if err := controllerClient.Create(ctx, sa); err != nil {
		t.Fatalf("expected managed backup ServiceAccount create to succeed, got: %v", err)
	}

	var latest corev1.ServiceAccount
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: sa.Name}, &latest); err != nil {
		t.Fatalf("get created ServiceAccount: %v", err)
	}
}

func TestVAP_ControllerServiceAccounts_AllowsCustomMainServiceAccountName(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	ensureControllerServiceAccountRBAC(t, namespace)
	controllerClient := newImpersonatedClient(t, controllerUsername)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-openbao-sa",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":           "openbao",
				"app.kubernetes.io/instance":       "example",
				"app.kubernetes.io/managed-by":     "openbao-operator",
				"openbao.org/cluster":              "example",
				"openbao.org/service-account-role": "main",
			},
		},
	}

	if err := controllerClient.Create(ctx, sa); err != nil {
		t.Fatalf("expected managed main ServiceAccount create to succeed, got: %v", err)
	}

	var latest corev1.ServiceAccount
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: sa.Name}, &latest); err != nil {
		t.Fatalf("get created ServiceAccount: %v", err)
	}
}
