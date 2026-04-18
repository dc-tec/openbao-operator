package openbaocluster

import (
	"errors"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
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

func (r *infraReconciler) newWorkloadManager() *workloadsvc.Manager {
	return workloadsvc.NewManager(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.Platform,
	).WithReader(r.deps.Kubernetes.APIReader)
}

func (r *infraReconciler) mapManagerReconcileError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, inframanager.ErrOIDCBootstrapAudienceMismatch):
		return operatorerrors.WithReason(constants.ReasonOIDCBootstrapConfigurationInvalid, err)
	case errors.Is(err, inframanager.ErrGatewayAPIMissing):
		return operatorerrors.WithReason(constants.ReasonGatewayAPIMissing, err)
	case errors.Is(err, inframanager.ErrAPIServerNetworkConfigurationInvalid):
		return operatorerrors.WithReason(constants.ReasonAPIServerNetworkConfigurationInvalid, err)
	case errors.Is(err, workloadsvc.ErrStatefulSetPrerequisitesMissing):
		return operatorerrors.WithReason(constants.ReasonPrerequisitesMissing, err)
	case errors.Is(err, inframanager.ErrACMEDomainNotResolvable):
		return operatorerrors.WithReason(constants.ReasonACMEDomainNotResolvable, err)
	case errors.Is(err, inframanager.ErrACMEGatewayNotConfiguredForPassthrough):
		return operatorerrors.WithReason(constants.ReasonACMEGatewayNotConfiguredForPassthrough, err)
	default:
		return err
	}
}
