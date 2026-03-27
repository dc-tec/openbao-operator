package openbaocluster

import (
	"errors"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

func oidcConfigForInfraManager(oidc *OIDCConfig) *portauth.OIDCConfig {
	if oidc == nil {
		return nil
	}

	return &portauth.OIDCConfig{
		IssuerURL:          oidc.IssuerURL,
		OIDCDiscoveryURL:   oidc.OIDCDiscoveryURL,
		OIDCDiscoveryCAPEM: oidc.OIDCDiscoveryCAPEM,
		JWKSURL:            oidc.JWKSURL,
		JWKSCAPEM:          oidc.JWKSCAPEM,
		JWKSKeys:           oidc.JWKSKeys,
	}
}

func (r *infraReconciler) newInfraManager(effectiveOIDC *OIDCConfig) *inframanager.Manager {
	return inframanager.NewManagerWithReaderAndOIDCConfig(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.APIReader,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.OperatorNamespace,
		oidcConfigForInfraManager(effectiveOIDC),
		r.deps.Kubernetes.Platform,
	)
}

func (r *infraReconciler) mapManagerReconcileError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, inframanager.ErrOIDCBootstrapAudienceMismatch):
		return operatorerrors.WithReason(r.reasons.oidcBootstrapConfigurationReason(), err)
	case errors.Is(err, inframanager.ErrGatewayAPIMissing):
		return operatorerrors.WithReason(r.reasons.gatewayAPIMissingReason(), err)
	case errors.Is(err, inframanager.ErrAPIServerNetworkConfigurationInvalid):
		return operatorerrors.WithReason(r.reasons.apiServerNetworkConfigurationReason(), err)
	case errors.Is(err, inframanager.ErrStatefulSetPrerequisitesMissing):
		return operatorerrors.WithReason(r.reasons.prerequisitesMissingReason(), err)
	case errors.Is(err, inframanager.ErrACMEDomainNotResolvable):
		return operatorerrors.WithReason(r.reasons.acmeDomainNotResolvableReason(), err)
	case errors.Is(err, inframanager.ErrACMEGatewayNotConfiguredForPassthrough):
		return operatorerrors.WithReason(r.reasons.acmeGatewayNotConfiguredReason(), err)
	default:
		return err
	}
}
