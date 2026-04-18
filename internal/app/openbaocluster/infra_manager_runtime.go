package openbaocluster

import (
	"errors"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	bootstrapmanager "github.com/dc-tec/openbao-operator/internal/service/bootstrap"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
	networkingmanager "github.com/dc-tec/openbao-operator/internal/service/networking"
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

func (r *infraReconciler) newBootstrapManager(effectiveOIDC *OIDCConfig) *bootstrapmanager.Manager {
	return bootstrapmanager.NewManagerWithReaderAndOIDCConfig(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.APIReader,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.OperatorNamespace,
		oidcConfigForInfraManager(effectiveOIDC),
	)
}

func (r *infraReconciler) newWorkloadManager() *workloadsvc.Manager {
	return workloadsvc.NewManager(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.Platform,
	).WithReader(r.deps.Kubernetes.APIReader)
}

func (r *infraReconciler) newNetworkingManager() *networkingmanager.Manager {
	return networkingmanager.NewManagerWithReader(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.APIReader,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.OperatorNamespace,
		r.deps.Kubernetes.Platform,
	)
}

func (r *infraReconciler) newInfraManager() *inframanager.Manager {
	return inframanager.NewManagerWithReader(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.APIReader,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.OperatorNamespace,
		r.deps.Kubernetes.Platform,
	)
}

func (r *infraReconciler) mapManagerReconcileError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, bootstrapmanager.ErrOIDCBootstrapAudienceMismatch):
		return operatorerrors.WithReason(constants.ReasonOIDCBootstrapConfigurationInvalid, err)
	case errors.Is(err, networkingmanager.ErrGatewayAPIMissing):
		return operatorerrors.WithReason(constants.ReasonGatewayAPIMissing, err)
	case errors.Is(err, networkingmanager.ErrAPIServerNetworkConfigurationInvalid):
		return operatorerrors.WithReason(constants.ReasonAPIServerNetworkConfigurationInvalid, err)
	case errors.Is(err, workloadsvc.ErrStatefulSetPrerequisitesMissing):
		return operatorerrors.WithReason(constants.ReasonPrerequisitesMissing, err)
	case errors.Is(err, networkingmanager.ErrACMEDomainNotResolvable):
		return operatorerrors.WithReason(constants.ReasonACMEDomainNotResolvable, err)
	case errors.Is(err, networkingmanager.ErrACMEGatewayNotConfiguredForPassthrough):
		return operatorerrors.WithReason(constants.ReasonACMEGatewayNotConfiguredForPassthrough, err)
	default:
		return err
	}
}
