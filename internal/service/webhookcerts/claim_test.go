package webhookcerts

import (
	"context"
	"encoding/base64"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPrepareClaimWebhookRuntimeCreatesSecretAndConfiguration(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	runtime, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-")
	if err != nil {
		t.Fatalf("PrepareClaimWebhookRuntime() error = %v", err)
	}
	if runtime.CertDir == "" {
		t.Fatalf("PrepareClaimWebhookRuntime() CertDir = empty, want non-empty")
	}

	secret, err := clientset.CoreV1().Secrets("openbao-operator-system").Get(context.Background(), "openbao-operator-controller-webhook-certs", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get Secret() error = %v", err)
	}
	if len(secret.Data[caCertKey]) == 0 || len(secret.Data[tlsCertKey]) == 0 || len(secret.Data[tlsKeyKey]) == 0 {
		t.Fatalf("Secret data = %#v, want CA and serving cert material", secret.Data)
	}

	config, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), "openbao-operator-openbaoclusterclaim-service-offering", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get MutatingWebhookConfiguration() error = %v", err)
	}
	if len(config.Webhooks) != 1 {
		t.Fatalf("webhooks = %d, want 1", len(config.Webhooks))
	}
	if got := string(config.Webhooks[0].ClientConfig.CABundle); got != string(secret.Data[caCertKey]) {
		t.Fatalf("caBundle mismatch between webhook configuration and Secret")
	}
}

func TestPrepareClaimWebhookRuntimeReusesExistingSecretBundle(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	first, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-")
	if err != nil {
		t.Fatalf("first PrepareClaimWebhookRuntime() error = %v", err)
	}
	if first.CertDir == "" {
		t.Fatalf("first CertDir = empty, want non-empty")
	}
	secretBefore, err := clientset.CoreV1().Secrets("openbao-operator-system").Get(context.Background(), "openbao-operator-controller-webhook-certs", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get Secret() error = %v", err)
	}
	caBefore := base64.StdEncoding.EncodeToString(secretBefore.Data[caCertKey])
	certBefore := base64.StdEncoding.EncodeToString(secretBefore.Data[tlsCertKey])

	second, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-")
	if err != nil {
		t.Fatalf("second PrepareClaimWebhookRuntime() error = %v", err)
	}
	if second.CertDir == "" {
		t.Fatalf("second CertDir = empty, want non-empty")
	}
	secretAfter, err := clientset.CoreV1().Secrets("openbao-operator-system").Get(context.Background(), "openbao-operator-controller-webhook-certs", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get Secret() after second reconcile error = %v", err)
	}
	if got := base64.StdEncoding.EncodeToString(secretAfter.Data[caCertKey]); got != caBefore {
		t.Fatalf("ca bundle rotated unexpectedly")
	}
	if got := base64.StdEncoding.EncodeToString(secretAfter.Data[tlsCertKey]); got != certBefore {
		t.Fatalf("serving certificate rotated unexpectedly")
	}
}

func TestPrepareClaimWebhookRuntimeDeletesConfigurationWhenDisabled(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "openbao-operator-controller-webhook-certs", Namespace: "openbao-operator-system"},
		},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "openbao-operator-openbaoclusterclaim-service-offering"},
		},
	)
	if _, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", false, "openbao-operator-"); err != nil {
		t.Fatalf("PrepareClaimWebhookRuntime() error = %v", err)
	}
	if _, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), "openbao-operator-openbaoclusterclaim-service-offering", metav1.GetOptions{}); err == nil {
		t.Fatalf("MutatingWebhookConfiguration still exists after disable")
	}
	if _, err := clientset.CoreV1().Secrets("openbao-operator-system").Get(context.Background(), "openbao-operator-controller-webhook-certs", metav1.GetOptions{}); err == nil {
		t.Fatalf("Secret still exists after disable")
	}
}

func TestReadAndValidateBundleRejectsWrongServingNamespace(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	if _, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-"); err != nil {
		t.Fatalf("PrepareClaimWebhookRuntime() error = %v", err)
	}
	secret, err := clientset.CoreV1().Secrets("openbao-operator-system").Get(context.Background(), "openbao-operator-controller-webhook-certs", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get Secret() error = %v", err)
	}
	if _, valid, err := readAndValidateBundle(secret, "other-namespace", "openbao-operator-controller-webhook", metav1.Now().UTC()); err == nil && valid {
		t.Fatalf("readAndValidateBundle() = valid for wrong namespace, want invalid")
	}
}

func TestPrepareClaimWebhookRuntimeUpdatesStaleConfiguration(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "openbao-operator-controller-webhook-certs", Namespace: "openbao-operator-system"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{},
		},
		&admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "openbao-operator-openbaoclusterclaim-service-offering"},
			Webhooks:   []admissionregistrationv1.MutatingWebhook{{Name: "wrong"}},
		},
	)
	if _, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-"); err != nil {
		t.Fatalf("PrepareClaimWebhookRuntime() error = %v", err)
	}
	config, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.Background(), "openbao-operator-openbaoclusterclaim-service-offering", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get MutatingWebhookConfiguration() error = %v", err)
	}
	if len(config.Webhooks) != 1 || config.Webhooks[0].Name != "mopenbaoclusterclaims.openbao.org" {
		t.Fatalf("webhooks = %#v, want rewritten claim webhook configuration", config.Webhooks)
	}
}
