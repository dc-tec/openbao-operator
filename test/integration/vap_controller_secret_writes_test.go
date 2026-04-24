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

const controllerUsername = "system:serviceaccount:openbao-operator-system:openbao-operator-controller"

func ensureControllerSecretRBAC(t *testing.T, namespace string) {
	t.Helper()

	ensureControllerResourceRBAC(t, namespace, "controller-secret-manager", "controller-secret-manager-binding", "secrets")
}

func TestVAP_ControllerSecretWrites_DeniesUnexpectedSecretName(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	ensureControllerSecretRBAC(t, namespace)
	controllerClient := newImpersonatedClient(t, controllerUsername)

	for attempt := 0; attempt < 25; attempt++ {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-secret",
				Namespace: namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "openbao-operator",
					"openbao.org/cluster":          "example",
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"token": []byte("test"),
			},
		}

		err := controllerClient.Create(ctx, secret)
		if err == nil {
			_ = k8sClient.Delete(ctx, secret)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "controller can only create, update, or delete operator-managed cluster Secrets") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny controller Secret creation with unexpected name after retries")
}

func TestVAP_ControllerSecretWrites_AllowsManagedRootTokenSecret(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	ensureControllerSecretRBAC(t, namespace)
	controllerClient := newImpersonatedClient(t, controllerUsername)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-root-token",
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao",
				"app.kubernetes.io/instance":   "example",
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/cluster":          "example",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("test"),
		},
	}

	if err := controllerClient.Create(ctx, secret); err != nil {
		t.Fatalf("expected controller managed Secret create to succeed, got: %v", err)
	}

	var latest corev1.Secret
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secret.Name}, &latest); err != nil {
		t.Fatalf("get created Secret: %v", err)
	}
}
