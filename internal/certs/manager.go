package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

const (
	caCertKey  = "ca.crt"
	caKeyKey   = "ca.key"
	tlsCertKey = "tls.crt"
	tlsKeyKey  = "tls.key"

	caCertValidityYears    = 10
	serverCertValidityDays = 365
)

// Manager reconciles TLS-related Kubernetes resources for an OpenBaoCluster.
// It is responsible for bootstrapping and maintaining per-cluster CA and server
// certificate Secrets.
type Manager struct {
	client   client.Client
	scheme   *runtime.Scheme
	reloader ReloadSignaler
}

// ReloadSignaler is responsible for triggering a TLS reload when the server certificate changes.
// Implementations may annotate pods or StatefulSets and send SIGHUP to OpenBao processes.
type ReloadSignaler interface {
	SignalReload(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, certHash string) error
}

type noopReloadSignaler struct{}

func (n noopReloadSignaler) SignalReload(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, certHash string) error {
	return nil
}

// NewManager constructs a Manager that uses the provided Kubernetes client and a no-op reload signaler.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return NewManagerWithReloader(c, scheme, nil)
}

// NewManagerWithReloader constructs a Manager with the provided client, scheme, and reload signaler.
// When reloader is nil, a no-op implementation is used.
// The scheme is used to set OwnerReferences on created resources for garbage collection.
func NewManagerWithReloader(c client.Client, scheme *runtime.Scheme, r ReloadSignaler) *Manager {
	if r == nil {
		r = noopReloadSignaler{}
	}
	return &Manager{
		client:   c,
		scheme:   scheme,
		reloader: r,
	}
}

// applySecret creates or patches a Secret using Server-Side Apply.
func (m *Manager) applySecret(ctx context.Context, secret *corev1.Secret) error {
	secret.TypeMeta = metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Secret",
	}
	return m.client.Patch(ctx, secret, client.Apply, client.FieldOwner("openbao-cert-manager"), client.ForceOwnership)
}

// Reconcile ensures TLS assets are aligned with the desired state for the given OpenBaoCluster.
//
// It bootstraps a per-cluster CA Secret and server certificate Secret, evaluates
// rotation based on the configured rotation window, and triggers hot-reload via the
// ReloadSignaler whenever a new server certificate is issued.
//
// When Mode is External, the operator does not generate or rotate certificates.
// It only waits for external Secrets to exist and triggers hot-reload when they change.
// When Mode is ACME, OpenBao manages certificates internally via its native ACME client.
//
//nolint:gocyclo // Reconcile handles multiple TLS modes and k8s object flows; kept linear for readability.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	if !cluster.Spec.TLS.Enabled {
		logger.Info("TLS is disabled for OpenBaoCluster; skipping certificate reconciliation")
		return recon.Result{}, nil
	}

	metrics := newTLSMetrics(cluster.Namespace, cluster.Name)

	// Determine TLS mode, defaulting to OperatorManaged for backwards compatibility
	mode := cluster.Spec.TLS.Mode
	if mode == "" {
		mode = openbaov1alpha1.TLSModeOperatorManaged
	}

	// Handle ACME mode: OpenBao manages certificates internally, no operator action needed
	if mode == openbaov1alpha1.TLSModeACME {
		logger.Info("TLS mode is ACME; OpenBao manages certificates internally, skipping operator reconciliation")
		return recon.Result{}, nil
	}

	// Handle External mode: wait for secrets and trigger reload on changes
	if mode == openbaov1alpha1.TLSModeExternal {
		shouldRequeue, err := m.reconcileExternalTLS(ctx, logger, cluster, metrics)
		if err != nil {
			return recon.Result{}, err
		}
		if shouldRequeue {
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}
		return recon.Result{}, nil
	}

	// OperatorManaged mode: generate and rotate certificates
	now := time.Now()

	caSecretName := caSecretName(cluster)
	caSecret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caSecretName,
	}, caSecret)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return recon.Result{}, fmt.Errorf("failed to get CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
		}

		logger.Info("CA Secret not found; generating new CA", "secret", caSecretName)

		caCertPEM, caKeyPEM, genErr := generateCA(cluster, now)
		if genErr != nil {
			return recon.Result{}, fmt.Errorf("failed to generate CA for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, genErr)
		}

		caSecret = buildCASecret(cluster, caSecretName, caCertPEM, caKeyPEM)

		// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
		if err := controllerutil.SetControllerReference(cluster, caSecret, m.scheme); err != nil {
			return recon.Result{}, fmt.Errorf("failed to set owner reference on CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
		}

		if err := m.applySecret(ctx, caSecret); err != nil {
			return recon.Result{}, fmt.Errorf("failed to apply CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
		}
	}

	caCert, caKey, caCertPEM, parseErr := parseCAFromSecret(caSecret)
	if parseErr != nil {
		return recon.Result{}, fmt.Errorf("failed to parse CA secret %s/%s: %w", cluster.Namespace, caSecretName, parseErr)
	}

	serverSecretName := serverSecretName(cluster)
	serverSecret := &corev1.Secret{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serverSecretName,
	}, serverSecret)
	var serverCertPEM []byte
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return recon.Result{}, fmt.Errorf("failed to get server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}

		logger.Info("Server TLS Secret not found; generating new server certificate", "secret", serverSecretName)

		// Note: We do not include Pod IPs in certificate SANs because Pod IPs are ephemeral
		// in Kubernetes. Every time a Pod is recreated (e.g., node drain, upgrade), its IP
		// changes, which would force unnecessary certificate rotation. We rely on stable DNS
		// entries provided by the StatefulSet (pod-ordinal.service-name.namespace.svc) and
		// the Service ClusterIP, which are already included in the certificate SANs.

		serverCertPEM, serverKeyPEM, issueErr := issueServerCertificate(cluster, caCert, caKey, now)
		if issueErr != nil {
			return recon.Result{}, fmt.Errorf("failed to issue server certificate for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, issueErr)
		}

		serverSecret = buildServerSecret(cluster, serverSecretName, serverCertPEM, serverKeyPEM, caCertPEM)

		// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
		if err := controllerutil.SetControllerReference(cluster, serverSecret, m.scheme); err != nil {
			return recon.Result{}, fmt.Errorf("failed to set owner reference on server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}

		if err := m.applySecret(ctx, serverSecret); err != nil {
			return recon.Result{}, fmt.Errorf("failed to apply server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}
		if err := m.signalReloadIfNeeded(ctx, logger, cluster, serverCertPEM); err != nil {
			return recon.Result{}, err
		}

		// Record expiry and initial rotation.
		metrics.setServerCertExpiry(now.AddDate(0, 0, serverCertValidityDays), "OperatorManaged")
		metrics.incrementRotation()

		return recon.Result{}, nil
	}

	serverCert, certErr := parseServerCertificateFromSecret(serverSecret)
	if certErr != nil {
		logger.Info("Existing server certificate could not be parsed; reissuing", "secret", serverSecretName)

		// Note: We do not include Pod IPs in certificate SANs because Pod IPs are ephemeral
		// in Kubernetes. Every time a Pod is recreated (e.g., node drain, upgrade), its IP
		// changes, which would force unnecessary certificate rotation. We rely on stable DNS
		// entries provided by the StatefulSet (pod-ordinal.service-name.namespace.svc) and
		// the Service ClusterIP, which are already included in the certificate SANs.

		serverCertPEM, serverKeyPEM, issueErr := issueServerCertificate(cluster, caCert, caKey, now)
		if issueErr != nil {
			return recon.Result{}, fmt.Errorf("failed to reissue server certificate for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, issueErr)
		}

		serverSecret = buildServerSecret(cluster, serverSecretName, serverCertPEM, serverKeyPEM, caCertPEM)
		if err := controllerutil.SetControllerReference(cluster, serverSecret, m.scheme); err != nil {
			return recon.Result{}, fmt.Errorf("failed to set owner reference on server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}

		if err := m.applySecret(ctx, serverSecret); err != nil {
			return recon.Result{}, fmt.Errorf("failed to apply server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}

		if err := m.signalReloadIfNeeded(ctx, logger, cluster, serverCertPEM); err != nil {
			return recon.Result{}, err
		}

		// Record expiry and rotation after issuing a replacement certificate.
		metrics.setServerCertExpiry(now.AddDate(0, 0, serverCertValidityDays), "OperatorManaged")
		metrics.incrementRotation()

		return recon.Result{}, nil
	}

	// Note: We do not include Pod IPs in certificate SANs because Pod IPs are ephemeral
	// in Kubernetes. Every time a Pod is recreated (e.g., node drain, upgrade), its IP
	// changes, which would force unnecessary certificate rotation. We rely on stable DNS
	// entries provided by the StatefulSet (pod-ordinal.service-name.namespace.svc) and
	// the Service ClusterIP, which are already included in the certificate SANs.

	// Check if certificate SANs match expected SANs; regenerate if they don't
	expectedDNS, expectedIPs, sansErr := buildServerSANs(cluster)
	if sansErr != nil {
		return recon.Result{}, fmt.Errorf("failed to compute expected server certificate SANs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, sansErr)
	}
	if !certSANsMatch(serverCert, expectedDNS, expectedIPs) {
		// Log detailed information about the mismatch for debugging
		certIPs := make([]string, 0, len(serverCert.IPAddresses))
		for _, ip := range serverCert.IPAddresses {
			if ip != nil {
				certIPs = append(certIPs, ip.String())
			}
		}
		expectedIPStrs := make([]string, 0, len(expectedIPs))
		for _, ip := range expectedIPs {
			if ip != nil {
				expectedIPStrs = append(expectedIPStrs, ip.String())
			}
		}
		logger.Info("Server certificate SANs do not match expected SANs; reissuing",
			"secret", serverSecretName,
			"certificate_ips", certIPs,
			"expected_ips", expectedIPStrs)

		serverCertPEM, serverKeyPEM, issueErr := issueServerCertificate(cluster, caCert, caKey, now)
		if issueErr != nil {
			return recon.Result{}, fmt.Errorf("failed to reissue server certificate for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, issueErr)
		}

		serverSecret = buildServerSecret(cluster, serverSecretName, serverCertPEM, serverKeyPEM, caCertPEM)
		if err := controllerutil.SetControllerReference(cluster, serverSecret, m.scheme); err != nil {
			return recon.Result{}, fmt.Errorf("failed to set owner reference on server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}

		if err := m.applySecret(ctx, serverSecret); err != nil {
			return recon.Result{}, fmt.Errorf("failed to apply server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
		}

		if err := m.signalReloadIfNeeded(ctx, logger, cluster, serverCertPEM); err != nil {
			return recon.Result{}, err
		}

		return recon.Result{}, nil
	}

	rotate, rotateErr := shouldRotateServerCert(serverCert, now, cluster.Spec.TLS.RotationPeriod)
	if rotateErr != nil {
		return recon.Result{}, fmt.Errorf("failed to evaluate rotation for server certificate %s/%s: %w", cluster.Namespace, serverSecretName, rotateErr)
	}

	if !rotate {
		// No rotation required; record current expiry from the existing certificate.
		metrics.setServerCertExpiry(serverCert.NotAfter, "OperatorManaged")
		return recon.Result{}, nil
	}

	logger.Info("Server certificate is within rotation window; reissuing", "secret", serverSecretName)

	// Note: We do not include Pod IPs in certificate SANs because Pod IPs are ephemeral
	// in Kubernetes. Every time a Pod is recreated (e.g., node drain, upgrade), its IP
	// changes, which would force unnecessary certificate rotation. We rely on stable DNS
	// entries provided by the StatefulSet (pod-ordinal.service-name.namespace.svc) and
	// the Service ClusterIP, which are already included in the certificate SANs.

	serverCertPEM, serverKeyPEM, issueErr := issueServerCertificate(cluster, caCert, caKey, now)
	if issueErr != nil {
		return recon.Result{}, fmt.Errorf("failed to rotate server certificate for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, issueErr)
	}

	serverSecret = buildServerSecret(cluster, serverSecretName, serverCertPEM, serverKeyPEM, caCertPEM)
	if err := controllerutil.SetControllerReference(cluster, serverSecret, m.scheme); err != nil {
		return recon.Result{}, fmt.Errorf("failed to set owner reference on server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
	}

	if err := m.applySecret(ctx, serverSecret); err != nil {
		return recon.Result{}, fmt.Errorf("failed to apply server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
	}

	if err := m.signalReloadIfNeeded(ctx, logger, cluster, serverCertPEM); err != nil {
		return recon.Result{}, err
	}

	// Record expiry and rotation for the newly issued certificate.
	metrics.setServerCertExpiry(now.AddDate(0, 0, serverCertValidityDays), "OperatorManaged")
	metrics.incrementRotation()

	return recon.Result{}, nil
}

// reconcileExternalTLS handles TLS reconciliation when Mode is External.
// It waits for external Secrets to exist and triggers hot-reload when certificates change.
func (m *Manager) reconcileExternalTLS(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *tlsMetrics) (bool, error) {
	caSecretName := caSecretName(cluster)
	caSecret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caSecretName,
	}, caSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Waiting for external TLS CA Secret", "secret", caSecretName)
			return true, nil
		}
		return false, fmt.Errorf("failed to get CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
	}

	serverSecretName := serverSecretName(cluster)
	serverSecret := &corev1.Secret{}
	err = m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      serverSecretName,
	}, serverSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Waiting for external TLS server Secret", "secret", serverSecretName)
			return true, nil
		}
		return false, fmt.Errorf("failed to get server TLS Secret %s/%s: %w", cluster.Namespace, serverSecretName, err)
	}

	// Both secrets exist. Calculate hash and trigger reload if needed.
	// This enables hot-reload when cert-manager or other external tools rotate certificates.
	serverCertPEM, ok := serverSecret.Data[tlsCertKey]
	if !ok || len(serverCertPEM) == 0 {
		logger.Info("External TLS server Secret exists but missing certificate; waiting for external provider to populate", "secret", serverSecretName)
		return true, nil
	}

	// For external TLS, parse the certificate to record its expiry time.
	serverCert, parseErr := parseCertificate(serverCertPEM)
	if parseErr == nil {
		metrics.setServerCertExpiry(serverCert.NotAfter, "External")
	}

	if err := m.signalReloadIfNeeded(ctx, logger, cluster, serverCertPEM); err != nil {
		return false, err
	}

	return false, nil
}

func caSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSCA
}

func serverSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func generateCA(cluster *openbaov1alpha1.OpenBaoCluster, now time.Time) ([]byte, []byte, error) {
	// Use ECDSA P-256 for better security and performance compared to RSA-2048
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	serialNumber, err := randSerialNumber()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s OpenBao Root CA", cluster.Name),
			Organization: []string{"OpenBao Operator"},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(caCertValidityYears, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

func buildCASecret(cluster *openbaov1alpha1.OpenBaoCluster, name string, certPEM []byte, keyPEM []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			caCertKey: certPEM,
			caKeyKey:  keyPEM,
		},
	}
}

func parseCAFromSecret(secret *corev1.Secret) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	certPEM, ok := secret.Data[caCertKey]
	if !ok || len(certPEM) == 0 {
		return nil, nil, nil, fmt.Errorf("missing %q in CA Secret", caCertKey)
	}

	keyPEM, ok := secret.Data[caKeyKey]
	if !ok || len(keyPEM) == 0 {
		return nil, nil, nil, fmt.Errorf("missing %q in CA Secret", caKeyKey)
	}

	cert, err := parseCertificate(certPEM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	privateKey, err := parseECDSAPrivateKey(keyPEM)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	return cert, privateKey, certPEM, nil
}

func issueServerCertificate(cluster *openbaov1alpha1.OpenBaoCluster, caCert *x509.Certificate, caKey *ecdsa.PrivateKey, now time.Time) ([]byte, []byte, error) {
	// Use ECDSA P-256 for better security and performance compared to RSA-2048
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate server private key: %w", err)
	}

	serialNumber, err := randSerialNumber()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate server certificate serial number: %w", err)
	}

	dnsNames, ipAddresses, sansErr := buildServerSANs(cluster)
	if sansErr != nil {
		return nil, nil, fmt.Errorf("failed to compute SANs for server certificate: %w", sansErr)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s.openbao.svc", cluster.Name),
			Organization: []string{"OpenBao Operator"},
		},
		NotBefore: now.Add(-1 * time.Hour),
		NotAfter:  now.AddDate(0, 0, serverCertValidityDays),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		// Include both ServerAuth and ClientAuth to support mutual TLS for retry_join
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

func buildServerSecret(cluster *openbaov1alpha1.OpenBaoCluster, name string, certPEM []byte, keyPEM []byte, caCertPEM []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cluster.Namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			tlsCertKey: certPEM,
			tlsKeyKey:  keyPEM,
			caCertKey:  caCertPEM,
		},
	}
}

func randSerialNumber() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	return serialNumber, nil
}

func parseCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

func parseECDSAPrivateKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode ECDSA private key PEM (expected EC PRIVATE KEY, got %q)", block.Type)
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECDSA private key: %w", err)
	}

	return privateKey, nil
}

// buildServerSANs builds the Subject Alternative Names (SANs) for the server certificate.
// It includes DNS names based on the cluster configuration and user-provided ExtraSANs.
// Pod IPs are intentionally excluded because they are ephemeral in Kubernetes and would
// cause unnecessary certificate rotation when pods are recreated.
func buildServerSANs(cluster *openbaov1alpha1.OpenBaoCluster) ([]string, []net.IP, error) {
	dnsSet := map[string]struct{}{}
	ipSet := map[string]struct{}{}

	addDNS := func(name string) {
		if strings.TrimSpace(name) == "" {
			return
		}
		if _, exists := dnsSet[name]; !exists {
			dnsSet[name] = struct{}{}
		}
	}

	addIP := func(ip net.IP) {
		if ip == nil {
			return
		}
		key := ip.String()
		if _, exists := ipSet[key]; !exists {
			ipSet[key] = struct{}{}
		}
	}

	namespace := strings.TrimSpace(cluster.Namespace)
	if namespace == "" {
		return nil, nil, fmt.Errorf("cluster namespace is required to build server certificate SANs")
	}

	clusterName := strings.TrimSpace(cluster.Name)

	addDNS("localhost")
	addIP(net.ParseIP("127.0.0.1"))

	// Add a common DNS SAN for auto-join with go-discover.
	// This common DNS name is used with leader_tls_servername in retry_join
	// to avoid certificate validation issues when go-discover returns IP addresses.
	// The DNS name does not need to be resolvable; it's just a common identifier
	// that all pods use for TLS validation during auto-join.
	if clusterName != "" {
		commonDNSName := fmt.Sprintf("openbao-cluster-%s.local", clusterName)
		addDNS(commonDNSName)
	}

	if clusterName != "" {
		// Add wildcards for all pods in the cluster
		// Include both .svc and .svc.cluster.local for comprehensive coverage
		addDNS(fmt.Sprintf("*.%s.%s.svc", clusterName, namespace))
		addDNS(fmt.Sprintf("*.%s.%s.svc.cluster.local", clusterName, namespace))
		// Add the headless service name
		addDNS(fmt.Sprintf("%s.%s.svc", clusterName, namespace))
		addDNS(fmt.Sprintf("%s.%s.svc.cluster.local", clusterName, namespace))
		// Add individual pod DNS names for explicit coverage
		// This ensures certificates work for retry_join even if wildcard matching has issues
		replicas := cluster.Spec.Replicas
		for i := int32(0); i < replicas; i++ {
			addDNS(fmt.Sprintf("%s-%d.%s.%s.svc", clusterName, i, clusterName, namespace))
			addDNS(fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local", clusterName, i, clusterName, namespace))
		}

		// For Blue/Green upgrades, explicitly add SANs for the revision-specific pod names.
		// Wildcards like *.bluegreen-cluster.svc work for standard pods, but for Green pods
		// like bluegreen-cluster-hash-0, we want to be explicit to ensure TLS validation works.
		if cluster.Status.BlueGreen != nil {
			revisions := []string{}
			if cluster.Status.BlueGreen.BlueRevision != "" {
				revisions = append(revisions, cluster.Status.BlueGreen.BlueRevision)
			}
			if cluster.Status.BlueGreen.GreenRevision != "" {
				revisions = append(revisions, cluster.Status.BlueGreen.GreenRevision)
			}

			for _, rev := range revisions {
				for i := int32(0); i < replicas; i++ {
					podName := fmt.Sprintf("%s-%s-%d", clusterName, rev, i)
					addDNS(fmt.Sprintf("%s.%s.%s.svc", podName, clusterName, namespace))
					addDNS(fmt.Sprintf("%s.%s.%s.svc.cluster.local", podName, clusterName, namespace))
				}
			}
		}
	}

	// Add namespace-wide wildcards for both DNS suffixes
	addDNS(fmt.Sprintf("*.%s.svc", namespace))
	addDNS(fmt.Sprintf("*.%s.svc.cluster.local", namespace))

	// Add external service DNS name if service, ingress, or gateway is configured
	// This is needed when ingress controllers or gateways connect to the backend service
	if clusterName != "" {
		needsExternalService := (cluster.Spec.Service != nil) ||
			(cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled) ||
			(cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled)
		if needsExternalService {
			externalServiceName := fmt.Sprintf("%s-public.%s.svc", clusterName, namespace)
			addDNS(externalServiceName)
		}
	}

	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled && strings.TrimSpace(cluster.Spec.Ingress.Host) != "" {
		addDNS(strings.TrimSpace(cluster.Spec.Ingress.Host))
	}

	// Add Gateway hostname to TLS SANs if Gateway API is enabled
	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled && strings.TrimSpace(cluster.Spec.Gateway.Hostname) != "" {
		addDNS(strings.TrimSpace(cluster.Spec.Gateway.Hostname))
	}

	for _, san := range cluster.Spec.TLS.ExtraSANs {
		if ip := net.ParseIP(san); ip != nil {
			addIP(ip)
			continue
		}
		addDNS(san)
	}

	// Note: Pod IPs are intentionally NOT included in certificate SANs because Pod IPs
	// are ephemeral in Kubernetes. Every time a Pod is recreated (e.g., node drain, upgrade),
	// its IP changes, which would force unnecessary certificate rotation. We rely on stable
	// DNS entries provided by the StatefulSet (pod-ordinal.service-name.namespace.svc) and
	// the Service ClusterIP, which are already included above.

	dnsNames := make([]string, 0, len(dnsSet))
	for name := range dnsSet {
		dnsNames = append(dnsNames, name)
	}

	ipAddresses := make([]net.IP, 0, len(ipSet))
	for key := range ipSet {
		ipAddresses = append(ipAddresses, net.ParseIP(key))
	}

	return dnsNames, ipAddresses, nil
}

func parseServerCertificateFromSecret(secret *corev1.Secret) (*x509.Certificate, error) {
	certPEM, ok := secret.Data[tlsCertKey]
	if !ok || len(certPEM) == 0 {
		return nil, fmt.Errorf("missing %q in server TLS Secret", tlsCertKey)
	}

	return parseCertificate(certPEM)
}

// certSANsMatch checks if the certificate contains all expected DNS names and IP addresses.
// It returns false if any expected SAN is missing from the certificate.
func certSANsMatch(cert *x509.Certificate, expectedDNS []string, expectedIPs []net.IP) bool {
	// Build sets of DNS names and IPs from the certificate
	certDNSSet := make(map[string]struct{})
	for _, dns := range cert.DNSNames {
		certDNSSet[dns] = struct{}{}
	}

	certIPSet := make(map[string]struct{})
	for _, ip := range cert.IPAddresses {
		if ip != nil {
			certIPSet[ip.String()] = struct{}{}
		}
	}

	// Check that all expected DNS names are present
	for _, expectedDNS := range expectedDNS {
		if _, found := certDNSSet[expectedDNS]; !found {
			return false
		}
	}

	// Check that all expected IP addresses are present
	for _, expectedIP := range expectedIPs {
		if expectedIP == nil {
			continue
		}
		if _, found := certIPSet[expectedIP.String()]; !found {
			return false
		}
	}

	return true
}

func shouldRotateServerCert(cert *x509.Certificate, now time.Time, rotationPeriod string) (bool, error) {
	if rotationPeriod == "" {
		return false, nil
	}

	duration, err := time.ParseDuration(rotationPeriod)
	if err != nil {
		return false, fmt.Errorf("invalid TLS rotation period %q: %w", rotationPeriod, err)
	}

	remaining := cert.NotAfter.Sub(now)
	return remaining < duration, nil
}

func (m *Manager) signalReloadIfNeeded(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, certPEM []byte) error {
	if len(certPEM) == 0 {
		return nil
	}

	sum := sha256.Sum256(certPEM)
	if m.reloader == nil {
		return nil
	}

	if err := m.reloader.SignalReload(ctx, logger, cluster, fmt.Sprintf("%x", sum[:])); err != nil {
		return fmt.Errorf("failed to signal TLS reload for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	return nil
}
