package webhookcerts

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	admissionregistrationv1client "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
)

const (
	claimWebhookCertFile    = "tls.crt"
	claimWebhookKeyFile     = "tls.key"
	claimWebhookCertBaseDir = "/tmp/k8s-webhook-server"
	claimWebhookPath        = "/mutate-openbao-org-v1alpha1-openbaoclusterclaim"
	claimWebhookPort        = int32(443)

	caCertKey  = "ca.crt"
	tlsCertKey = "tls.crt"
	tlsKeyKey  = "tls.key"

	caValidityYears       = 10
	servingValidityDays   = 365
	servingRotationWindow = 30 * 24 * time.Hour
)

type ClaimWebhookRuntime struct {
	CertDir string
}

type ClaimWebhookResourceNames struct {
	SecretName               string
	ServiceName              string
	WebhookConfigurationName string
}

func PrepareClaimWebhookRuntime(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	enabled bool,
	namePrefix string,
) (ClaimWebhookRuntime, error) {
	names := claimWebhookResourceNames(namePrefix)
	if !enabled {
		if err := deleteClaimWebhookSecret(ctx, clientset.CoreV1().Secrets(namespace), names.SecretName); err != nil {
			return ClaimWebhookRuntime{}, err
		}
		if err := deleteClaimWebhookConfiguration(ctx, clientset.AdmissionregistrationV1().MutatingWebhookConfigurations(), names.WebhookConfigurationName); err != nil {
			return ClaimWebhookRuntime{}, err
		}
		return ClaimWebhookRuntime{}, nil
	}

	bundle, err := ensureClaimWebhookSecret(ctx, clientset.CoreV1().Secrets(namespace), namespace, names)
	if err != nil {
		return ClaimWebhookRuntime{}, err
	}
	if err := ensureClaimWebhookConfiguration(ctx, clientset.AdmissionregistrationV1().MutatingWebhookConfigurations(), namespace, bundle.caPEM, names); err != nil {
		return ClaimWebhookRuntime{}, err
	}
	certDir, err := writeServingCertDir(bundle.tlsPEM, bundle.keyPEM)
	if err != nil {
		return ClaimWebhookRuntime{}, err
	}
	return ClaimWebhookRuntime{CertDir: certDir}, nil
}

func claimWebhookResourceNames(namePrefix string) ClaimWebhookResourceNames {
	prefix := strings.TrimSpace(namePrefix)
	if prefix == "" {
		prefix = "openbao-operator-"
	}
	if !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}
	return ClaimWebhookResourceNames{
		SecretName:               prefix + "controller-webhook-certs",
		ServiceName:              prefix + "controller-webhook",
		WebhookConfigurationName: prefix + "openbaoclusterclaim-service-offering",
	}
}

type certBundle struct {
	caPEM  []byte
	tlsPEM []byte
	keyPEM []byte
}

type generatedBundle struct {
	caPEM  []byte
	tlsPEM []byte
	keyPEM []byte
}

func ensureClaimWebhookSecret(
	ctx context.Context,
	secrets corev1client.SecretInterface,
	namespace string,
	names ClaimWebhookResourceNames,
) (certBundle, error) {
	now := time.Now().UTC()
	var result certBundle
	if err := retry.OnError(retry.DefaultBackoff, isRetryableWebhookRuntimeMutationError, func() error {
		secret, getErr := secrets.Get(ctx, names.SecretName, metav1.GetOptions{})
		if getErr == nil {
			bundle, valid, validateErr := readAndValidateBundle(secret, namespace, names.ServiceName, now)
			if validateErr == nil && valid {
				result = bundle
				return nil
			}
		}
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			return fmt.Errorf("get claim webhook TLS Secret %s/%s: %w", namespace, names.SecretName, getErr)
		}

		bundle, err := generateBundle(namespace, names.ServiceName, now)
		if err != nil {
			return err
		}
		desired := desiredClaimWebhookSecret(namespace, names, bundle)
		if apierrors.IsNotFound(getErr) {
			if _, err := secrets.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
				if isRetryableWebhookRuntimeMutationError(err) {
					return err
				}
				return fmt.Errorf("create claim webhook TLS Secret %s/%s: %w", namespace, names.SecretName, err)
			}
			result = certBundle(bundle)
			return nil
		}
		secret.Type = desired.Type
		secret.Labels = desired.Labels
		secret.Data = desired.Data
		if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			if isRetryableWebhookRuntimeMutationError(err) {
				return err
			}
			return fmt.Errorf("update claim webhook TLS Secret %s/%s: %w", namespace, names.SecretName, err)
		}
		result = certBundle(bundle)
		return nil
	}); err != nil {
		return certBundle{}, fmt.Errorf("reconcile claim webhook TLS Secret %s/%s: %w", namespace, names.SecretName, err)
	}
	return result, nil
}

func desiredClaimWebhookSecret(namespace string, names ClaimWebhookResourceNames, bundle generatedBundle) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SecretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao-operator",
				"app.kubernetes.io/component":  "controller-webhook",
				"app.kubernetes.io/managed-by": "openbao-operator",
				"openbao.org/component":        "controller-webhook",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			caCertKey:  bundle.caPEM,
			tlsCertKey: bundle.tlsPEM,
			tlsKeyKey:  bundle.keyPEM,
		},
	}
}

func isRetryableWebhookRuntimeMutationError(err error) bool {
	return apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err)
}

func readAndValidateBundle(secret *corev1.Secret, namespace, serviceName string, now time.Time) (certBundle, bool, error) {
	if secret == nil {
		return certBundle{}, false, nil
	}
	caPEM := secret.Data[caCertKey]
	tlsPEM := secret.Data[tlsCertKey]
	keyPEM := secret.Data[tlsKeyKey]
	if len(caPEM) == 0 || len(tlsPEM) == 0 || len(keyPEM) == 0 {
		return certBundle{}, false, nil
	}
	caCert, err := parseCertificate(caPEM)
	if err != nil {
		return certBundle{}, false, err
	}
	servingCert, err := parseCertificate(tlsPEM)
	if err != nil {
		return certBundle{}, false, err
	}
	if _, err := tls.X509KeyPair(tlsPEM, keyPEM); err != nil {
		return certBundle{}, false, err
	}
	if servingCert.NotAfter.Before(now.Add(servingRotationWindow)) {
		return certBundle{}, false, nil
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(caPEM)
	if _, err := servingCert.Verify(x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: now,
		DNSName:     claimWebhookCommonName(namespace, serviceName),
	}); err != nil {
		return certBundle{}, false, nil
	}
	if !bytes.Equal(servingCert.RawIssuer, caCert.RawSubject) {
		return certBundle{}, false, nil
	}
	return certBundle{caPEM: caPEM, tlsPEM: tlsPEM, keyPEM: keyPEM}, true, nil
}

func generateBundle(namespace, serviceName string, now time.Time) (generatedBundle, error) {
	caCertPEM, caCert, caKey, err := generateCA(now)
	if err != nil {
		return generatedBundle{}, err
	}
	tlsCertPEM, tlsKeyPEM, err := generateServingCert(namespace, serviceName, now, caCert, caKey)
	if err != nil {
		return generatedBundle{}, err
	}
	return generatedBundle{caPEM: caCertPEM, tlsPEM: tlsCertPEM, keyPEM: tlsKeyPEM}, nil
}

func generateCA(now time.Time) ([]byte, *x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate claim webhook CA key: %w", err)
	}
	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate claim webhook CA serial: %w", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "openbao-operator-webhook-ca"},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(caValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create claim webhook CA certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse claim webhook CA certificate: %w", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return caPEM, cert, key, nil
}

func generateServingCert(namespace, serviceName string, now time.Time, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate claim webhook serving key: %w", err)
	}
	serial, err := randomSerialNumber()
	if err != nil {
		return nil, nil, fmt.Errorf("generate claim webhook serving serial: %w", err)
	}
	altNames := []string{
		serviceName,
		fmt.Sprintf("%s.%s", serviceName, namespace),
		claimWebhookCommonName(namespace, serviceName),
	}
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: claimWebhookCommonName(namespace, serviceName)},
		NotBefore:    now.Add(-1 * time.Hour),
		NotAfter:     now.AddDate(0, 0, servingValidityDays),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     altNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create claim webhook serving certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal claim webhook serving key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

func ensureClaimWebhookConfiguration(
	ctx context.Context,
	configs admissionregistrationv1client.MutatingWebhookConfigurationInterface,
	namespace string,
	caPEM []byte,
	names ClaimWebhookResourceNames,
) error {
	if err := retry.OnError(retry.DefaultBackoff, isRetryableWebhookRuntimeMutationError, func() error {
		desired := desiredClaimWebhookConfiguration(namespace, caPEM, names)
		existing, err := configs.Get(ctx, names.WebhookConfigurationName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err := configs.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
				if isRetryableWebhookRuntimeMutationError(err) {
					return err
				}
				return fmt.Errorf("create claim mutating webhook configuration %s: %w", names.WebhookConfigurationName, err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("get claim mutating webhook configuration %s: %w", names.WebhookConfigurationName, err)
		}
		existing.Labels = desired.Labels
		existing.Webhooks = desired.Webhooks
		if _, err := configs.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			if isRetryableWebhookRuntimeMutationError(err) {
				return err
			}
			return fmt.Errorf("update claim mutating webhook configuration %s: %w", names.WebhookConfigurationName, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("reconcile claim mutating webhook configuration %s: %w", names.WebhookConfigurationName, err)
	}
	return nil
}

func desiredClaimWebhookConfiguration(
	namespace string,
	caPEM []byte,
	names ClaimWebhookResourceNames,
) *admissionregistrationv1.MutatingWebhookConfiguration {
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: names.WebhookConfigurationName,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "openbao-operator",
				"app.kubernetes.io/component":  "controller",
				"app.kubernetes.io/managed-by": "openbao-operator",
			},
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			Name:                    "mopenbaoclusterclaims.openbao.org",
			AdmissionReviewVersions: []string{"v1"},
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				CABundle: caPEM,
				Service: &admissionregistrationv1.ServiceReference{
					Name:      names.ServiceName,
					Namespace: namespace,
					Path:      ptr.To(claimWebhookPath),
					Port:      ptr.To(claimWebhookPort),
				},
			},
			FailurePolicy:      ptr.To(admissionregistrationv1.Fail),
			MatchPolicy:        ptr.To(admissionregistrationv1.Equivalent),
			ReinvocationPolicy: ptr.To(admissionregistrationv1.NeverReinvocationPolicy),
			SideEffects:        ptr.To(admissionregistrationv1.SideEffectClassNone),
			TimeoutSeconds:     ptr.To[int32](10),
			Rules: []admissionregistrationv1.RuleWithOperations{{
				Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
				Rule: admissionregistrationv1.Rule{
					APIGroups:   []string{"openbao.org"},
					APIVersions: []string{"v1alpha1"},
					Resources:   []string{"openbaoclusterclaims"},
					Scope:       ptr.To(admissionregistrationv1.NamespacedScope),
				},
			}},
		}},
	}
}

func deleteClaimWebhookConfiguration(
	ctx context.Context,
	configs admissionregistrationv1client.MutatingWebhookConfigurationInterface,
	name string,
) error {
	if err := configs.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete claim mutating webhook configuration %s: %w", name, err)
	}
	return nil
}

func deleteClaimWebhookSecret(
	ctx context.Context,
	secrets corev1client.SecretInterface,
	name string,
) error {
	if err := secrets.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete claim webhook TLS Secret %s: %w", name, err)
	}
	return nil
}

func writeServingCertDir(certPEM, keyPEM []byte) (string, error) {
	if err := os.MkdirAll(claimWebhookCertBaseDir, 0o700); err != nil {
		return "", fmt.Errorf("prepare claim webhook cert base dir: %w", err)
	}
	dir, err := os.MkdirTemp(claimWebhookCertBaseDir, "openbao-claim-webhook-")
	if err != nil {
		return "", fmt.Errorf("create claim webhook cert dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, claimWebhookCertFile), certPEM, 0o600); err != nil {
		return "", fmt.Errorf("write claim webhook cert: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, claimWebhookKeyFile), keyPEM, 0o600); err != nil {
		return "", fmt.Errorf("write claim webhook key: %w", err)
	}
	return dir, nil
}

func claimWebhookCommonName(namespace, serviceName string) string {
	return fmt.Sprintf("%s.%s.svc", serviceName, namespace)
}

func randomSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func parseCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}
