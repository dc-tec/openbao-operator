package auth

import "fmt"

const (
	defaultKubernetesOIDCDiscoveryBaseURL = "https://kubernetes.default.svc"
	oidcWellKnownConfigurationPath        = "/.well-known/openid-configuration"
	kubernetesOIDCJWKSPath                = "/openid/v1/jwks"
	legacyOIDCJWKSPath                    = "/.well-known/jwks.json"
	publicKeyPEMBlockType                 = "PUBLIC KEY"
	serviceAccountCAPath                  = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// HTTPStatusError represents a non-200 response when calling an HTTP endpoint.
// It allows callers to make decisions (e.g. retry vs. fail-fast) based on status code.
type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	if e.URL == "" {
		return fmt.Sprintf("endpoint returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("%s returned status %d", e.URL, e.StatusCode)
}

// HTTPStatusCode returns the HTTP status associated with the discovery failure.
func (e *HTTPStatusError) HTTPStatusCode() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}
