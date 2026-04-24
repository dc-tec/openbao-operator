package openbaoclusterclaim

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestApplySecretWithFallbackAppliesUnownedSecret(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "projected-auth-config",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"config.json": []byte(`{"issuer":"https://issuer.example.internal"}`)},
	}

	updated := secret.DeepCopy()
	updated.Data = map[string][]byte{"config.json": []byte(`{"issuer":"https://issuer.example.internal","audience":"openbao"}`)}

	if err := applySecretWithFallback(context.Background(), c, nil, nil, secret); err != nil {
		t.Fatalf("first applySecretWithFallback() error = %v", err)
	}
	if err := applySecretWithFallback(context.Background(), c, nil, nil, secret.DeepCopy()); err != nil {
		t.Fatalf("second applySecretWithFallback() error = %v", err)
	}
	if err := applySecretWithFallback(context.Background(), c, nil, nil, updated); err != nil {
		t.Fatalf("third applySecretWithFallback() error = %v", err)
	}

	current := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(secret), current); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got := string(current.Data["config.json"]); got != `{"issuer":"https://issuer.example.internal","audience":"openbao"}` {
		t.Fatalf("stored secret data = %q, want updated payload", got)
	}
}

func TestApplySecretWithFallbackAppliesOwnedSecret(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(openbaov1alpha1) error = %v", err)
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "openbao-operator-system",
			Name:      "payments-bao",
			UID:       "claim-uid",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(claim).Build()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      "payments-bao-connection",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"endpoint": []byte("https://payments-bao.payments.svc:8200"),
			"ca.crt":   []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
		},
	}

	if err := applySecretWithFallback(context.Background(), c, scheme, claim, secret); err != nil {
		t.Fatalf("first applySecretWithFallback() error = %v", err)
	}
	if err := applySecretWithFallback(context.Background(), c, scheme, claim, secret.DeepCopy()); err != nil {
		t.Fatalf("second applySecretWithFallback() error = %v", err)
	}

	current := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(secret), current); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(current.OwnerReferences) != 1 || current.OwnerReferences[0].Name != claim.Name {
		t.Fatalf("OwnerReferences = %#v, want controller reference to claim %q", current.OwnerReferences, claim.Name)
	}
}
