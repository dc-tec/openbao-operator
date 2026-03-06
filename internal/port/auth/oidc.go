package auth

import (
	"context"
	"errors"

	"k8s.io/client-go/rest"
)

// OIDCConfig contains discovered issuer and key material for JWT bootstrap.
type OIDCConfig struct {
	IssuerURL string
	JWKSKeys  []string
}

// DiscoverConfigFunc discovers OIDC configuration for a Kubernetes API server.
type DiscoverConfigFunc func(ctx context.Context, cfg *rest.Config, baseURL string) (*OIDCConfig, error)

// DiscoveryStatusCodeFunc extracts HTTP status codes from discovery failures.
type DiscoveryStatusCodeFunc func(err error) (int, bool)

type httpStatusCoder interface {
	error
	HTTPStatusCode() int
}

// DiscoveryStatusCode extracts an HTTP status code from an OIDC discovery error when available.
func DiscoveryStatusCode(err error) (int, bool) {
	var statusErr httpStatusCoder
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatusCode(), true
	}
	return 0, false
}
