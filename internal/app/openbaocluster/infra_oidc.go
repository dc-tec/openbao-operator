package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

// OIDCConfig contains discovered issuer and key material for JWT bootstrap.
type OIDCConfig struct {
	IssuerURL          string
	OIDCDiscoveryURL   string
	OIDCDiscoveryCAPEM string
	JWKSURL            string
	JWKSCAPEM          string
	JWKSKeys           []string
}

func (r *infraReconciler) oidcBootstrapConfigurationError(err error) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
		err = operatorerrors.WrapPermanentConfig(err)
	}
	return operatorerrors.WithReason(constants.ReasonOIDCBootstrapConfigurationInvalid, err)
}

func shouldBootstrapJWTAuth(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return portauth.OperatorJWTBootstrapEnabled(cluster)
}

func (r *infraReconciler) oidcDiscoveryStatusCode(err error) (int, bool) {
	if r == nil || r.deps.OIDC.DiscoveryStatusCode == nil {
		return 0, false
	}
	return r.deps.OIDC.DiscoveryStatusCode(err)
}

func (r *infraReconciler) oidcDiscoveryError(err error) error {
	if err == nil {
		return nil
	}

	if statusCode, ok := r.oidcDiscoveryStatusCode(err); ok {
		switch statusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return r.oidcBootstrapConfigurationError(fmt.Errorf(
				"OIDC discovery blocked by Kubernetes API RBAC (%d). Ensure the operator ServiceAccount can GET %q and %q on the Kubernetes API server (nonResourceURLs RBAC): %w",
				statusCode,
				"/.well-known/openid-configuration",
				"/openid/v1/jwks",
				err,
			))
		case http.StatusNotFound:
			return r.oidcBootstrapConfigurationError(fmt.Errorf(
				"OIDC discovery endpoint not found (404). Ensure the Kubernetes API server exposes OIDC discovery and JWKS endpoints: %w",
				err,
			))
		default:
			if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
				return operatorerrors.WrapTransientKubernetesAPI(err)
			}
			return r.oidcBootstrapConfigurationError(err)
		}
	}

	if operatorerrors.IsTransientConnection(err) {
		return operatorerrors.WrapTransientKubernetesAPI(operatorerrors.WrapTransientConnection(err))
	}

	if errors.Is(err, portauth.ErrDiscoveryContentInvalid) {
		return r.oidcBootstrapConfigurationError(err)
	}

	return operatorerrors.WrapTransientKubernetesAPI(operatorerrors.WrapTransientConnection(err))
}

func (r *infraReconciler) resolveOIDC(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*OIDCConfig, error) {
	effective := &OIDCConfig{
		IssuerURL:          r.deps.OIDC.OIDCIssuer,
		OIDCDiscoveryURL:   r.deps.OIDC.OIDCDiscoveryURL,
		OIDCDiscoveryCAPEM: r.deps.OIDC.OIDCDiscoveryCAPEM,
		JWKSURL:            r.deps.OIDC.OIDCJWKSURL,
		JWKSCAPEM:          r.deps.OIDC.OIDCJWKSCAPEM,
		JWKSKeys:           append([]string(nil), r.deps.OIDC.OIDCJWTKeys...),
	}

	if !shouldBootstrapJWTAuth(cluster) || (strings.TrimSpace(effective.IssuerURL) != "" && (strings.TrimSpace(effective.OIDCDiscoveryURL) != "" || strings.TrimSpace(effective.JWKSURL) != "" || len(effective.JWKSKeys) > 0)) {
		return effective, nil
	}

	if r.deps.OIDC.RestConfig == nil {
		return nil, r.oidcBootstrapConfigurationError(fmt.Errorf("OIDC discovery required but controller rest.Config is not available"))
	}

	discover := r.deps.OIDC.DiscoverOIDCConfig
	if discover == nil {
		return nil, r.oidcBootstrapConfigurationError(fmt.Errorf("OIDC discovery function is not configured"))
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	discovered, err := discover(discoveryCtx, r.deps.OIDC.RestConfig)
	if err != nil {
		return nil, r.oidcDiscoveryError(err)
	}
	if discovered == nil || strings.TrimSpace(discovered.IssuerURL) == "" {
		return nil, r.oidcBootstrapConfigurationError(fmt.Errorf("OIDC discovery returned empty issuer"))
	}
	if strings.TrimSpace(discovered.OIDCDiscoveryURL) == "" && strings.TrimSpace(discovered.JWKSURL) == "" && len(discovered.JWKSKeys) == 0 {
		return nil, r.oidcBootstrapConfigurationError(fmt.Errorf("OIDC discovery returned no JWT validation material"))
	}

	return discovered, nil
}
