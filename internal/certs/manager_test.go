package certs

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestSecretNames(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	tests := []struct {
		name     string
		fn       func(*openbaov1alpha1.OpenBaoCluster) string
		expected string
	}{
		{
			name:     "CA secret name suffix",
			fn:       caSecretName,
			expected: "test-cluster-tls-ca",
		},
		{
			name:     "Server secret name suffix",
			fn:       serverSecretName,
			expected: "test-cluster-tls-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.fn(cluster)
			if got != tt.expected {
				t.Fatalf("got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildServerSANsIncludesDefaultsAndExtraSANs(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prod-cluster",
			Namespace: "security",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
				ExtraSANs: []string{
					"10.0.0.10",
				},
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
		},
	}

	host := "bao.example.com"
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    host,
	}

	dnsNames, ipAddresses, err := buildServerSANs(cluster)
	if err != nil {
		t.Fatalf("buildServerSANs() error = %v", err)
	}

	expectDNS := map[string]bool{
		"*.prod-cluster.security.svc":        false,
		"prod-cluster.security.svc":          false,
		"*.security.svc":                     false,
		"localhost":                          false,
		"bao.example.com":                    false,
		"openbao-cluster-prod-cluster.local": false,
	}

	for _, name := range dnsNames {
		if _, ok := expectDNS[name]; ok {
			expectDNS[name] = true
		}
	}

	for name, seen := range expectDNS {
		if !seen {
			t.Fatalf("expected DNS SAN %q to be present", name)
		}
	}

	foundIP := false
	for _, ip := range ipAddresses {
		if ip != nil && ip.String() == "10.0.0.10" {
			foundIP = true
			break
		}
	}
	if !foundIP {
		t.Fatalf("expected IP SAN 10.0.0.10 to be present")
	}
}

func TestShouldRotateServerCert(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		notAfterOffset time.Duration
		rotation       string
		wantRotate     bool
		wantErr        bool
	}{
		{
			name:           "no rotation when far from expiry",
			notAfterOffset: 48 * time.Hour,
			rotation:       "24h",
			wantRotate:     false,
			wantErr:        false,
		},
		{
			name:           "rotate when within window",
			notAfterOffset: 6 * time.Hour,
			rotation:       "24h",
			wantRotate:     true,
			wantErr:        false,
		},
		{
			name:           "error for invalid rotation period",
			notAfterOffset: 24 * time.Hour,
			rotation:       "not-a-duration",
			wantRotate:     false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cert := &openbaov1alpha1.OpenBaoCluster{}
			_ = cert

			x509Cert := &x509.Certificate{
				NotAfter: now.Add(tt.notAfterOffset),
			}

			gotRotate, err := shouldRotateServerCert(x509Cert, now, tt.rotation)
			if (err != nil) != tt.wantErr {
				t.Fatalf("shouldRotateServerCert() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && gotRotate != tt.wantRotate {
				t.Fatalf("shouldRotateServerCert() = %v, want %v", gotRotate, tt.wantRotate)
			}
		})
	}
}

func TestReconcileCreatesCAAndServerSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add OpenBao scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	manager := NewManager(client, scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev-cluster",
			Namespace: "security",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.1.0",
			Image:    "openbao/openbao:2.1.0",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}

	err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	caSecret := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caSecretName(cluster),
	}, caSecret)
	if err != nil {
		t.Fatalf("expected CA Secret to exist: %v", err)
	}

	if len(caSecret.Data[caCertKey]) == 0 {
		t.Fatalf("expected CA Secret to contain %q", caCertKey)
	}
	if len(caSecret.Data[caKeyKey]) == 0 {
		t.Fatalf("expected CA Secret to contain %q", caKeyKey)
	}

	serverSecret := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serverSecretName(cluster),
	}, serverSecret)
	if err != nil {
		t.Fatalf("expected server TLS Secret to exist: %v", err)
	}

	if len(serverSecret.Data[tlsCertKey]) == 0 {
		t.Fatalf("expected server TLS Secret to contain %q", tlsCertKey)
	}
	if len(serverSecret.Data[tlsKeyKey]) == 0 {
		t.Fatalf("expected server TLS Secret to contain %q", tlsKeyKey)
	}
	if len(serverSecret.Data[caCertKey]) == 0 {
		t.Fatalf("expected server TLS Secret to contain %q", caCertKey)
	}
}

type recordingReloadSignaler struct {
	called   bool
	lastHash string
}

func (r *recordingReloadSignaler) SignalReload(_ context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, certHash string) error {
	r.called = true
	r.lastHash = certHash
	return nil
}

func TestReconcileTriggersReloadOnNewServerCert(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add OpenBao scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reloader := &recordingReloadSignaler{}
	manager := NewManagerWithReloader(client, scheme, reloader)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reload-cluster",
			Namespace: "security",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.1.0",
			Image:    "openbao/openbao:2.1.0",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}

	err := manager.Reconcile(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !reloader.called {
		t.Fatalf("expected reload signaler to be called on initial server certificate creation")
	}
	if reloader.lastHash == "" {
		t.Fatalf("expected non-empty certificate hash from reload signaler")
	}
}

func TestReconcileRotatesNearExpiryCertAndSignalsReload(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add OpenBao scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	reloader := &recordingReloadSignaler{}
	manager := NewManagerWithReloader(client, scheme, reloader)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rotate-cluster",
			Namespace: "security",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "24h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}

	ctx := context.Background()

	// First reconcile bootstraps CA and server certificate and triggers a reload.
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("initial Reconcile() error = %v", err)
	}
	if !reloader.called || reloader.lastHash == "" {
		t.Fatalf("expected initial reload signaler call with non-empty hash")
	}

	initialHash := reloader.lastHash
	reloader.called = false
	reloader.lastHash = ""

	// Fetch CA and server Secrets so we can replace the server certificate with one
	// that is near expiry and should trigger rotation.
	caSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caSecretName(cluster),
	}, caSecret); err != nil {
		t.Fatalf("expected CA Secret to exist: %v", err)
	}

	serverSecret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serverSecretName(cluster),
	}, serverSecret); err != nil {
		t.Fatalf("expected server Secret to exist: %v", err)
	}

	caCert, caKey, _, err := parseCAFromSecret(caSecret)
	if err != nil {
		t.Fatalf("parseCAFromSecret() error = %v", err)
	}

	// Issue a new server certificate that expires soon, so that the configured
	// rotation period ("24h") will consider it within the rotation window.
	now := time.Now()
	serial, err := randSerialNumber()
	if err != nil {
		t.Fatalf("randSerialNumber() error = %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "rotate-cluster.openbao.svc",
			Organization: []string{"OpenBao Operator"},
		},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.Add(6 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &privKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create near-expiry server certificate: %v", err)
	}

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	serverSecret.Data[tlsCertKey] = serverCertPEM
	if err := client.Update(ctx, serverSecret); err != nil {
		t.Fatalf("failed to update server Secret with near-expiry cert: %v", err)
	}

	// Second reconcile should detect that the certificate is within the rotation
	// window, rotate it, and signal reload with a new hash.
	if err := manager.Reconcile(ctx, logr.Discard(), cluster); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}

	if !reloader.called {
		t.Fatalf("expected reload signaler to be called after rotation")
	}
	if reloader.lastHash == "" {
		t.Fatalf("expected non-empty certificate hash after rotation")
	}
	if reloader.lastHash == initialHash {
		t.Fatalf("expected certificate hash to change after rotation")
	}
}

func TestReconcileExternalModeWaitsForSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add OpenBao scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	manager := NewManager(client, scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-cluster",
			Namespace: "security",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				Mode:           openbaov1alpha1.TLSModeExternal,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}

	ctx := context.Background()

	// First reconcile should wait for secrets (no error, but no secrets created)
	err := manager.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("Reconcile() error = %v (should wait for external secrets)", err)
	}

	// Verify no secrets were created
	caSecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caSecretName(cluster),
	}, caSecret)
	if err == nil {
		t.Fatalf("expected CA Secret to not exist in External mode")
	}

	serverSecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serverSecretName(cluster),
	}, serverSecret)
	if err == nil {
		t.Fatalf("expected server TLS Secret to not exist in External mode")
	}
}

func TestReconcileExternalModeTriggersReloadOnExistingSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add OpenBao scheme: %v", err)
	}

	// Create external secrets manually (simulating cert-manager or user-provided)
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-cluster-tls-ca",
			Namespace: "security",
		},
		Data: map[string][]byte{
			caCertKey: []byte("fake-ca-cert"),
			caKeyKey:  []byte("fake-ca-key"),
		},
	}

	serverSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-cluster-tls-server",
			Namespace: "security",
		},
		Data: map[string][]byte{
			tlsCertKey: []byte("fake-server-cert"),
			tlsKeyKey:  []byte("fake-server-key"),
			caCertKey:  []byte("fake-ca-cert"),
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(caSecret, serverSecret).Build()

	reloader := &recordingReloadSignaler{}
	manager := NewManagerWithReloader(client, scheme, reloader)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-cluster",
			Namespace: "security",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				Mode:           openbaov1alpha1.TLSModeExternal,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}

	ctx := context.Background()

	// Reconcile should find the secrets and trigger reload
	err := manager.Reconcile(ctx, logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify reload was triggered (for hot-reload when cert-manager rotates certs)
	if !reloader.called {
		t.Fatalf("expected reload signaler to be called when external secrets exist")
	}
	if reloader.lastHash == "" {
		t.Fatalf("expected non-empty certificate hash from reload signaler")
	}

	// Verify secrets were not modified
	updatedServerSecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serverSecretName(cluster),
	}, updatedServerSecret)
	if err != nil {
		t.Fatalf("expected server Secret to still exist: %v", err)
	}

	if string(updatedServerSecret.Data[tlsCertKey]) != "fake-server-cert" {
		t.Fatalf("expected external server Secret to not be modified by operator")
	}
}
