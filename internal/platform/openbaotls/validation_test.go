package openbaotls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestValidateExternalServerSecret(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeExternal,
			},
		},
	}

	t.Run("valid", func(t *testing.T) {
		caSecret, serverSecret := newExternalTLSValidationSecrets(t, "openbao-cluster-example.local")
		if err := ValidateExternalServerSecret(cluster, caSecret, serverSecret); err != nil {
			t.Fatalf("ValidateExternalServerSecret() error = %v", err)
		}
	})

	t.Run("missing required dns san", func(t *testing.T) {
		caSecret, serverSecret := newExternalTLSValidationSecrets(t, "wrong-name.local")
		err := ValidateExternalServerSecret(cluster, caSecret, serverSecret)
		if err == nil || err.Error() == "" {
			t.Fatal("expected SAN validation error")
		}
	})
}

func newExternalTLSValidationSecrets(t *testing.T, dnsName string) (*corev1.Secret, *corev1.Secret) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(ca) error = %v", err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-ca",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(ca) error = %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(server) error = %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "test-server",
		},
		NotBefore:   now.Add(-time.Hour),
		NotAfter:    now.Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    []string{dnsName},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(server) error = %v", err)
	}
	serverPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey() error = %v", err)
	}
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "example-tls-ca", Namespace: "default"},
		Data: map[string][]byte{
			"ca.crt": caPEM,
		},
	}
	serverSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "example-tls-server", Namespace: "default"},
		Data: map[string][]byte{
			"tls.crt": serverPEM,
			"tls.key": serverKeyPEM,
			"ca.crt":  caPEM,
		},
	}
	return caSecret, serverSecret
}
