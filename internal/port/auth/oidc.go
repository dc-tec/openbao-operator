package auth

import (
	"context"
	"errors"

	"k8s.io/client-go/rest"
)

var (
	// ErrDiscoveryContentInvalid indicates OIDC discovery or JWKS content was syntactically valid enough to fetch
	// but semantically unusable for JWT bootstrap.
	ErrDiscoveryContentInvalid = errors.New("OIDC discovery content invalid")
)

// OIDCConfig contains discovered issuer and JWT validation material for
// operator bootstrap. OIDCDiscoveryURL is preferred for dynamic verification
// and key rotation; JWKSURL and JWKSKeys are retained as compatibility
// fallbacks when a discovery URL cannot be determined safely.
type OIDCConfig struct {
	IssuerURL          string
	OIDCDiscoveryURL   string
	OIDCDiscoveryCAPEM string
	JWKSURL            string
	JWKSCAPEM          string
	JWKSKeys           []string
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
