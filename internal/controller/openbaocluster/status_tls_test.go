package openbaocluster

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSetTLSReadyCondition(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	validCASecret, validServerSecret := newTLSReadyTestSecrets(t)

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		objects       []runtime.Object
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "tls disabled",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.TLS.Enabled = false
				return cluster
			}(),
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonDisabled,
			wantMessageIn: "disabled",
		},
		{
			name: "acme mode",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
				return cluster
			}(),
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    reasonUnknown,
			wantMessageIn: "ACME",
		},
		{
			name:          "missing secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonTLSSecretMissing,
			wantMessageIn: "not present yet",
		},
		{
			name:          "invalid secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			objects:       []runtime.Object{validCASecret.DeepCopy(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example" + constants.SuffixTLSServer, Namespace: "default"}, Data: map[string][]byte{"tls.crt": []byte("cert")}}},
			wantStatus:    metav1.ConditionFalse,
			wantReason:    ReasonTLSSecretInvalid,
			wantMessageIn: "Server TLS Secret is invalid",
		},
		{
			name:          "valid secret",
			cluster:       newOpenBaoClusterStatusTestObject(),
			objects:       []runtime.Object{validCASecret.DeepCopy(), validServerSecret.DeepCopy()},
			wantStatus:    metav1.ConditionTrue,
			wantReason:    reasonReady,
			wantMessageIn: "provisioned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithRuntimeObjects(obj)
			}
			reconciler := &OpenBaoClusterReconciler{Client: builder.Build()}

			reconciler.setTLSReadyCondition(context.Background(), tt.cluster)
			cond := meta.FindStatusCondition(tt.cluster.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			if cond == nil {
				t.Fatal("expected TLSReady condition")
			}
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("TLSReady condition = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
			}
			if tt.wantMessageIn != "" && !strings.Contains(cond.Message, tt.wantMessageIn) {
				t.Fatalf("message = %q, want substring %q", cond.Message, tt.wantMessageIn)
			}
		})
	}
}

func newTLSReadyTestSecrets(t *testing.T) (*corev1.Secret, *corev1.Secret) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
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
		DNSNames:    []string{"openbao-cluster-example.local"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example" + constants.SuffixTLSCA,
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": caPEM,
		},
	}
	serverSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example" + constants.SuffixTLSServer,
			Namespace: "default",
		},
		Data: map[string][]byte{
			"tls.crt": serverPEM,
			"tls.key": serverKeyPEM,
			"ca.crt":  caPEM,
		},
	}

	return caSecret, serverSecret
}
