package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

type caMaterial struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte
}

// reconcileOperatorManagedTLS handles the OperatorManaged TLS mode reconciliation.
// additionalDNSNames are DNS names computed by the controller layer (e.g., upgrade-strategy-specific pod names).
func (m *Manager) reconcileOperatorManagedTLS(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *tlsMetrics, additionalDNSNames []string) (recon.Result, error) {
	now := time.Now()

	ca, err := m.loadOrCreateCA(ctx, logger, cluster, now)
	if err != nil {
		return recon.Result{}, err
	}

	serverSecretName := serverSecretName(cluster)
	serverSecret, found, err := m.getSecret(ctx, cluster.Namespace, serverSecretName, "server TLS Secret")
	if err != nil {
		return recon.Result{}, err
	}
	if !found {
		logger.Info("Server TLS Secret not found; generating new server certificate", "secret", serverSecretName)
		if err := m.issueAndApplyServerSecret(ctx, logger, cluster, ca, serverSecretName, now, additionalDNSNames, "issue server certificate"); err != nil {
			return recon.Result{}, err
		}

		metrics.setServerCertExpiry(now.AddDate(0, 0, serverCertValidityDays), "OperatorManaged")
		metrics.incrementRotation()
		return recon.Result{}, nil
	}

	if err := m.ensureManagedSecretMetadata(ctx, cluster, serverSecret); err != nil {
		return recon.Result{}, err
	}

	return m.reconcileExistingOperatorManagedServerSecret(ctx, logger, cluster, metrics, ca, serverSecret, serverSecretName, now, additionalDNSNames)
}

func (m *Manager) loadOrCreateCA(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, now time.Time) (caMaterial, error) {
	caSecretName := caSecretName(cluster)
	caSecret, found, err := m.getSecret(ctx, cluster.Namespace, caSecretName, "CA Secret")
	if err != nil {
		return caMaterial{}, err
	}

	if !found {
		logger.Info("CA Secret not found; generating new CA", "secret", caSecretName)

		caCertPEM, caKeyPEM, genErr := generateCA(cluster, now)
		if genErr != nil {
			return caMaterial{}, fmt.Errorf("failed to generate CA for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, genErr)
		}

		caSecret = buildCASecret(cluster, caSecretName, caCertPEM, caKeyPEM)
		if err := m.applyOwnedSecret(ctx, cluster, caSecret, "CA Secret"); err != nil {
			return caMaterial{}, err
		}
	} else {
		if err := m.ensureManagedSecretMetadata(ctx, cluster, caSecret); err != nil {
			return caMaterial{}, err
		}
	}

	caCert, caKey, caCertPEM, parseErr := parseCAFromSecret(caSecret)
	if parseErr != nil {
		return caMaterial{}, fmt.Errorf("failed to parse CA secret %s/%s: %w", cluster.Namespace, caSecretName, parseErr)
	}

	return caMaterial{
		cert:    caCert,
		key:     caKey,
		certPEM: caCertPEM,
	}, nil
}

func (m *Manager) reconcileExistingOperatorManagedServerSecret(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *tlsMetrics,
	ca caMaterial,
	serverSecret *corev1.Secret,
	serverSecretName string,
	now time.Time,
	additionalDNSNames []string,
) (recon.Result, error) {
	serverCert, certErr := parseServerCertificateFromSecret(serverSecret)
	if certErr != nil {
		logger.Info("Existing server certificate could not be parsed; reissuing", "secret", serverSecretName)
		if err := m.issueAndApplyServerSecret(ctx, logger, cluster, ca, serverSecretName, now, additionalDNSNames, "reissue server certificate"); err != nil {
			return recon.Result{}, err
		}

		metrics.setServerCertExpiry(now.AddDate(0, 0, serverCertValidityDays), "OperatorManaged")
		metrics.incrementRotation()
		return recon.Result{}, nil
	}

	// Check if certificate SANs match expected SANs; regenerate if they don't.
	expectedDNS, expectedIPs, sansErr := buildServerSANs(cluster, additionalDNSNames)
	if sansErr != nil {
		return recon.Result{}, fmt.Errorf("failed to compute expected server certificate SANs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, sansErr)
	}
	if !certSANsMatch(serverCert, expectedDNS, expectedIPs) {
		logServerSANMismatch(logger, serverSecretName, serverCert, expectedIPs)
		if err := m.issueAndApplyServerSecret(ctx, logger, cluster, ca, serverSecretName, now, additionalDNSNames, "reissue server certificate"); err != nil {
			return recon.Result{}, err
		}
		return recon.Result{}, nil
	}

	rotate, rotateErr := shouldRotateServerCert(serverCert, now, cluster.Spec.TLS.RotationPeriod)
	if rotateErr != nil {
		return recon.Result{}, fmt.Errorf("failed to evaluate rotation for server certificate %s/%s: %w", cluster.Namespace, serverSecretName, rotateErr)
	}

	if !rotate {
		metrics.setServerCertExpiry(serverCert.NotAfter, "OperatorManaged")
		return recon.Result{}, nil
	}

	logger.Info("Server certificate is within rotation window; reissuing", "secret", serverSecretName)
	if err := m.issueAndApplyServerSecret(ctx, logger, cluster, ca, serverSecretName, now, additionalDNSNames, "rotate server certificate"); err != nil {
		return recon.Result{}, err
	}

	metrics.setServerCertExpiry(now.AddDate(0, 0, serverCertValidityDays), "OperatorManaged")
	metrics.incrementRotation()
	return recon.Result{}, nil
}

func (m *Manager) issueAndApplyServerSecret(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	ca caMaterial,
	serverSecretName string,
	now time.Time,
	additionalDNSNames []string,
	action string,
) error {
	// Note: We do not include Pod IPs in certificate SANs because Pod IPs are ephemeral
	// in Kubernetes. Every time a Pod is recreated (e.g., node drain, upgrade), its IP
	// changes, which would force unnecessary certificate rotation. We rely on stable DNS
	// entries provided by the StatefulSet (pod-ordinal.service-name.namespace.svc) and
	// the Service ClusterIP, which are already included in the certificate SANs.
	serverCertPEM, serverKeyPEM, err := issueServerCertificate(cluster, ca.cert, ca.key, now, additionalDNSNames)
	if err != nil {
		return fmt.Errorf("failed to %s for OpenBaoCluster %s/%s: %w", action, cluster.Namespace, cluster.Name, err)
	}

	serverSecret := buildServerSecret(cluster, serverSecretName, serverCertPEM, serverKeyPEM, ca.certPEM)
	if err := m.applyOwnedSecret(ctx, cluster, serverSecret, "server TLS Secret"); err != nil {
		return err
	}
	if err := m.signalReloadIfNeeded(ctx, logger, cluster, serverCertPEM); err != nil {
		return err
	}
	return nil
}

func logServerSANMismatch(logger logr.Logger, serverSecretName string, cert *x509.Certificate, expectedIPs []net.IP) {
	certIPs := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
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
}
