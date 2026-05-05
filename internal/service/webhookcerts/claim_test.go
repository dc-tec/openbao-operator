package webhookcerts

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPrepareClaimWebhookRuntimeCreatesSecretAndConfiguration(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	preparedRuntime, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-")
	if err != nil {
		t.Fatalf("PrepareClaimWebhookRuntime() error = %v", err)
	}
	if preparedRuntime.CertDir == "" {
		t.Fatalf("PrepareClaimWebhookRuntime() CertDir = empty, want non-empty")
	}

	secret, err := clientset.CoreV1().Secrets("openbao-operator-system").Get(context.Background(), "openbao-operator-controller-webhook-certs", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get Secret() error = %v", err)
	}
	if len(secret.Data[caCertKey]) == 0 || len(secret.Data[tlsCertKey]) == 0 || len(secret.Data[tlsKeyKey]) == 0 {
		t.Fatalf("Secret data = %#v, want CA and serving cert material", secret.Data)
	}
	if _, ok := secret.Data["ca.key"]; ok {
		t.Fatalf("Secret data contains ca.key, want only public CA bundle and serving cert material")
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

func TestPrepareClaimWebhookRuntimeToleratesCreateRaces(t *testing.T) {
	t.Parallel()

	clientset := k8sfake.NewSimpleClientset()
	secretCreateRaced := false
	webhookCreateRaced := false
	clientset.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if secretCreateRaced {
			return false, nil, nil
		}
		secretCreateRaced = true
		createAction := action.(k8stesting.CreateAction)
		secret := createAction.GetObject().(*corev1.Secret).DeepCopy()
		if err := clientset.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), secret, secret.Namespace); err != nil {
			return true, nil, err
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, secret.Name)
	})
	clientset.PrependReactor("create", "mutatingwebhookconfigurations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if webhookCreateRaced {
			return false, nil, nil
		}
		webhookCreateRaced = true
		createAction := action.(k8stesting.CreateAction)
		config := createAction.GetObject().(*admissionregistrationv1.MutatingWebhookConfiguration).DeepCopy()
		if err := clientset.Tracker().Create(admissionregistrationv1.SchemeGroupVersion.WithResource("mutatingwebhookconfigurations"), config, ""); err != nil {
			return true, nil, err
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{
			Group:    admissionregistrationv1.GroupName,
			Resource: "mutatingwebhookconfigurations",
		}, config.Name)
	})

	if _, err := PrepareClaimWebhookRuntime(context.Background(), clientset, "openbao-operator-system", true, "openbao-operator-"); err != nil {
		t.Fatalf("PrepareClaimWebhookRuntime() error = %v", err)
	}
	if !secretCreateRaced {
		t.Fatalf("Secret create race reactor was not exercised")
	}
	if !webhookCreateRaced {
		t.Fatalf("MutatingWebhookConfiguration create race reactor was not exercised")
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
	updateAttempts := 0
	clientset.PrependReactor("update", "mutatingwebhookconfigurations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		updateAttempts++
		if updateAttempts == 1 {
			return true, nil, apierrors.NewConflict(schema.GroupResource{
				Group:    admissionregistrationv1.GroupName,
				Resource: "mutatingwebhookconfigurations",
			}, "openbao-operator-openbaoclusterclaim-service-offering", errors.New("stale resource version"))
		}
		return false, nil, nil
	})
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
	if updateAttempts != 2 {
		t.Fatalf("MutatingWebhookConfiguration update attempts = %d, want 2", updateAttempts)
	}
}
