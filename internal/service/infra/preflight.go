package infra

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

var (
	ErrACMEDomainNotResolvable                = errors.New("acme domain not resolvable")
	ErrACMEGatewayNotConfiguredForPassthrough = errors.New("gateway not configured for acme passthrough")
)

const (
	acmePreflightDNSTimeout = 2 * time.Second
)

func (m *Manager) runACMEPreflight(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if !usesACMEMode(cluster) {
		return nil
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		if !cluster.Spec.Gateway.TLSPassthrough {
			return fmt.Errorf(
				"%w: tls.mode=ACME requires spec.gateway.tlsPassthrough=true (TLSRoute passthrough); HTTPRoute termination prevents OpenBao from completing ACME challenges",
				ErrACMEGatewayNotConfiguredForPassthrough,
			)
		}

		if err := m.validateGatewayPassthroughListener(ctx, cluster); err != nil {
			return err
		}
	}

	// The most painful ACME failures happen when scaling beyond 1 replica; do the
	// (potentially slow) DNS check only when it is most impactful.
	if cluster.Status.Initialized && cluster.Spec.Replicas > 1 {
		if err := m.preflightACMEDomainResolvability(ctx, logger, cluster); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) preflightACMEDomainResolvability(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.TLS.ACME == nil {
		return nil
	}
	if cluster.Spec.Configuration == nil || strings.TrimSpace(cluster.Spec.Configuration.ACMECARoot) == "" {
		// Only assert resolvability for private ACME CAs running inside the cluster.
		// For public ACME CAs, cluster-local DNS resolvability is not a reliable signal.
		return nil
	}

	var failures []string
	for _, domain := range acmeDomains(cluster) {
		resolveCtx, cancel := context.WithTimeout(ctx, acmePreflightDNSTimeout)
		_, err := net.DefaultResolver.LookupHost(resolveCtx, domain)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", domain, err))
		}
	}

	if len(failures) > 0 {
		logger.Info("ACME domain resolvability preflight failed", "failures", failures)
		return fmt.Errorf(
			"%w: one or more tls.acme domains do not resolve via cluster DNS (required for private ACME CAs): %s",
			ErrACMEDomainNotResolvable,
			strings.Join(failures, "; "),
		)
	}

	return nil
}

func (m *Manager) validateGatewayPassthroughListener(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled || !cluster.Spec.Gateway.TLSPassthrough {
		return nil
	}

	gwNS := cluster.Spec.Gateway.GatewayRef.Namespace
	if strings.TrimSpace(gwNS) == "" {
		gwNS = cluster.Namespace
	}

	gw := &gatewayv1.Gateway{}
	if err := m.reader.Get(ctx, types.NamespacedName{Namespace: gwNS, Name: cluster.Spec.Gateway.GatewayRef.Name}, gw); err != nil {
		if operatorerrors.IsCRDMissingError(err) {
			return ErrGatewayAPIMissing
		}
		if apierrors.IsNotFound(err) {
			return fmt.Errorf(
				"%w: referenced Gateway %s/%s not found (required for tlsPassthrough)",
				ErrACMEGatewayNotConfiguredForPassthrough,
				gwNS,
				cluster.Spec.Gateway.GatewayRef.Name,
			)
		}
		return fmt.Errorf("failed to get referenced Gateway %s/%s: %w", gwNS, cluster.Spec.Gateway.GatewayRef.Name, err)
	}

	var listeners []gatewayv1.Listener
	if ln := strings.TrimSpace(cluster.Spec.Gateway.ListenerName); ln != "" {
		for i := range gw.Spec.Listeners {
			if string(gw.Spec.Listeners[i].Name) == ln {
				listeners = append(listeners, gw.Spec.Listeners[i])
			}
		}
		if len(listeners) == 0 {
			return fmt.Errorf(
				"%w: referenced Gateway %s/%s does not have listener %q",
				ErrACMEGatewayNotConfiguredForPassthrough,
				gwNS,
				gw.Name,
				ln,
			)
		}
	} else {
		listeners = gw.Spec.Listeners
	}

	for i := range listeners {
		l := listeners[i]
		if l.Protocol != gatewayv1.TLSProtocolType {
			continue
		}
		if l.TLS == nil || l.TLS.Mode == nil || *l.TLS.Mode != gatewayv1.TLSModePassthrough {
			continue
		}
		return nil
	}

	return fmt.Errorf(
		"%w: referenced Gateway %s/%s does not have a TLS listener in Passthrough mode (listenerName=%q)",
		ErrACMEGatewayNotConfiguredForPassthrough,
		gwNS,
		gw.Name,
		cluster.Spec.Gateway.ListenerName,
	)
}
