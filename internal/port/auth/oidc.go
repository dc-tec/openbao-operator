package auth

import (
	"context"
	"errors"

	"k8s.io/client-go/rest"

	internalauth "github.com/dc-tec/openbao-operator/internal/adapter/auth"
)

// OIDCConfig contains discovered issuer and key material for JWT bootstrap.
type OIDCConfig struct {
	IssuerURL string
	JWKSKeys  []string
}

// DiscoverConfig fetches Kubernetes OIDC discovery configuration.
func DiscoverConfig(ctx context.Context, cfg *rest.Config, baseURL string) (*OIDCConfig, error) {
	discovered, err := internalauth.DiscoverConfig(ctx, cfg, baseURL)
	if err != nil {
		return nil, err
	}
	if discovered == nil {
		return nil, nil
	}

	return &OIDCConfig{
		IssuerURL: discovered.IssuerURL,
		JWKSKeys:  discovered.JWKSKeys,
	}, nil
}

// DiscoveryStatusCode extracts an HTTP status code from an OIDC discovery error when available.
func DiscoveryStatusCode(err error) (int, bool) {
	var statusErr *internalauth.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode, true
	}
	return 0, false
}
