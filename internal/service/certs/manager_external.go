package certs

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
)

// reconcileExternalTLS handles TLS reconciliation when Mode is External.
// It waits for external Secrets to exist and triggers hot-reload when certificates change.
func (m *Manager) reconcileExternalTLS(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, metrics *tlsMetrics) (bool, error) {
	caSecretName := caSecretName(cluster)
	caSecret, found, err := m.getSecret(ctx, cluster.Namespace, caSecretName, "CA Secret")
	if err != nil {
		return false, err
	}
	if !found {
		logger.Info("Waiting for external TLS CA Secret", "secret", caSecretName)
		return true, nil
	}

	serverSecretName := serverSecretName(cluster)
	serverSecret, found, err := m.getSecret(ctx, cluster.Namespace, serverSecretName, "server TLS Secret")
	if err != nil {
		return false, err
	}
	if !found {
		logger.Info("Waiting for external TLS server Secret", "secret", serverSecretName)
		return true, nil
	}

	if err := openbaotls.ValidateExternalServerSecret(cluster, caSecret, serverSecret); err != nil {
		logger.Info("External TLS assets are not usable yet; waiting for external provider to populate valid material", "error", err.Error())
		return true, nil
	}

	// Both secrets exist and are usable. Calculate hash and trigger reload if needed.
	// This enables hot-reload when cert-manager or other external tools rotate certificates.
	serverCertPEM := serverSecret.Data[tlsCertKey]

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
