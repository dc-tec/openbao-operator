package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"k8s.io/client-go/rest"

	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

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

// DiscoverConfig fetches the Kubernetes OIDC issuer configuration from the Kubernetes API server.
// baseURL allows tests (or specialized environments) to override the default
// Kubernetes API DNS name. When empty, it defaults to:
//
//	https://kubernetes.default.svc
//
// Returns an error if discovery fails. The operator can still run for Development
// profile clusters without OIDC, but Hardened profile requires OIDC.
func DiscoverConfig(ctx context.Context, cfg *rest.Config, baseURL string) (*portauth.OIDCConfig, error) {
	if baseURL == "" {
		baseURL = defaultKubernetesOIDCDiscoveryBaseURL
	}
	wellKnownURL := baseURL + oidcWellKnownConfigurationPath

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	oidcConfig, err := fetchOIDCWellKnown(ctx, httpClient, wellKnownURL)
	if err != nil {
		if statusErr, ok := err.(*HTTPStatusError); ok {
			return nil, statusErr
		}
		return nil, err
	}

	if oidcConfig.Issuer == "" {
		return nil, fmt.Errorf("OIDC config missing issuer")
	}

	issuerURL := oidcConfig.Issuer

	// Fetch JWKS keys if JWKS URI is available
	var (
		jwksKeys       []string
		effective      string
		jwksCAPEM      string
		discoveryURL   string
		discoveryCAPEM string
	)
	if oidcConfig.JWKSURI != "" {
		keys, err := FetchJWKSKeys(ctx, cfg, oidcConfig.JWKSURI)
		if err == nil {
			jwksKeys = keys
			jwksCAPEM = clusterTransportJWKSConfig(cfg, oidcConfig.JWKSURI)
			discoveryURL, discoveryCAPEM = dynamicOIDCDiscoveryConfig(baseURL, issuerURL, oidcConfig.JWKSURI, jwksCAPEM)
			effective = canonicalClusterJWKSURL(baseURL, oidcConfig.JWKSURI)
			if discoveryURL != "" {
				effective = ""
				jwksCAPEM = ""
			}
		}
		if err != nil {
			if fallbackURL, ok := issuerJWKSRediscoveryURL(ctx, issuerURL); ok {
				keys, err = fetchJWKSKeysPublic(ctx, fallbackURL)
				if err == nil {
					jwksKeys = keys
					discoveryURL = issuerURL
					discoveryCAPEM = ""
					effective = ""
					jwksCAPEM = ""
				}
			}
		}
		if err != nil {
			if fallbackURL, ok := kubernetesJWKSFallbackURL(baseURL, oidcConfig.JWKSURI); ok {
				keys, err = FetchJWKSKeys(ctx, cfg, fallbackURL)
				if err == nil {
					jwksKeys = keys
					discoveryURL = canonicalClusterDiscoveryURL(baseURL)
					discoveryCAPEM = clusterTransportJWKSConfig(cfg, fallbackURL)
					effective = ""
					jwksCAPEM = ""
				}
			}
		}
		if err != nil {
			// Log but don't fail - JWKS keys are optional for some configurations
			return &portauth.OIDCConfig{
				IssuerURL:          issuerURL,
				OIDCDiscoveryURL:   "",
				OIDCDiscoveryCAPEM: "",
				JWKSURL:            "",
				JWKSCAPEM:          "",
				JWKSKeys:           nil,
			}, fmt.Errorf("failed to fetch JWKS keys: %w", err)
		}
	}

	return &portauth.OIDCConfig{
		IssuerURL:          issuerURL,
		OIDCDiscoveryURL:   discoveryURL,
		OIDCDiscoveryCAPEM: discoveryCAPEM,
		JWKSURL:            effective,
		JWKSCAPEM:          jwksCAPEM,
		JWKSKeys:           jwksKeys,
	}, nil
}

type oidcWellKnownDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`

	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`

	N string `json:"n,omitempty"`
	E string `json:"e,omitempty"`

	X5c []string `json:"x5c,omitempty"`
}

// FetchJWKSKeys fetches and parses JWKS keys from the provided JWKS URI.
func FetchJWKSKeys(ctx context.Context, cfg *rest.Config, jwksURL string) ([]string, error) {
	if jwksURL == "" {
		return nil, fmt.Errorf("jwks URL is required")
	}

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	return fetchJWKSKeysWithClient(ctx, httpClient, jwksURL)
}

func fetchJWKSKeysPublic(ctx context.Context, jwksURL string) ([]string, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return fetchJWKSKeysWithClient(ctx, httpClient, jwksURL)
}

func fetchJWKSKeysWithClient(ctx context.Context, httpClient *http.Client, jwksURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create jwks request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jwks endpoint: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_ = err
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{URL: jwksURL, StatusCode: resp.StatusCode}
	}

	var jwks jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to parse jwks document: %w", err)
	}

	keys, err := pemPublicKeysFromJWKS(jwks)
	if err != nil {
		return nil, fmt.Errorf("failed to extract public keys from jwks: %w", err)
	}

	return keys, nil
}

func fetchOIDCWellKnown(ctx context.Context, httpClient *http.Client, wellKnownURL string) (*oidcWellKnownDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC discovery request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC well-known endpoint: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			_ = err
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{URL: wellKnownURL, StatusCode: resp.StatusCode}
	}

	var oidcConfig oidcWellKnownDocument
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return nil, err
	}

	return &oidcConfig, nil
}

func issuerJWKSRediscoveryURL(ctx context.Context, issuerURL string) (string, bool) {
	if issuerURL == "" {
		return "", false
	}

	issuer, err := url.Parse(issuerURL)
	if err != nil || issuer.Scheme == "" || issuer.Host == "" {
		return "", false
	}

	wellKnownURL := strings.TrimRight(issuerURL, "/") + oidcWellKnownConfigurationPath
	oidcConfig, err := fetchOIDCWellKnown(ctx, &http.Client{Timeout: 10 * time.Second}, wellKnownURL)
	if err != nil || oidcConfig == nil || oidcConfig.JWKSURI == "" {
		return "", false
	}

	return oidcConfig.JWKSURI, true
}

func clusterTransportJWKSConfig(cfg *rest.Config, jwksURL string) string {
	if cfg == nil || jwksURL == "" {
		return ""
	}

	jwks, err := url.Parse(jwksURL)
	if err != nil || jwks.Scheme == "" || jwks.Host == "" {
		return ""
	}
	if jwks.Scheme != "https" {
		return ""
	}

	if len(cfg.CAData) > 0 {
		return string(cfg.CAData)
	}

	if caFile := strings.TrimSpace(cfg.CAFile); caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err == nil {
			return string(caPEM)
		}
	}

	caPEM, err := os.ReadFile(serviceAccountCAPath)
	if err != nil {
		return ""
	}
	return string(caPEM)
}

func dynamicOIDCDiscoveryConfig(baseURL, issuerURL, jwksURL, caPEM string) (string, string) {
	if caPEM != "" {
		if discoveryURL := canonicalClusterDiscoveryURL(baseURL); discoveryURL != "" {
			if isStandardKubernetesJWKSURL(jwksURL) {
				return discoveryURL, caPEM
			}
		}
	}

	if canUseIssuerDiscoveryURL(issuerURL, jwksURL) {
		return issuerURL, ""
	}

	return "", ""
}

func canUseIssuerDiscoveryURL(issuerURL, jwksURL string) bool {
	if issuerURL == "" || jwksURL == "" {
		return false
	}

	issuer, err := url.Parse(issuerURL)
	if err != nil || issuer.Scheme == "" || issuer.Host == "" {
		return false
	}
	jwks, err := url.Parse(jwksURL)
	if err != nil || jwks.Scheme == "" || jwks.Host == "" {
		return false
	}

	return strings.EqualFold(issuer.Scheme, jwks.Scheme) && strings.EqualFold(issuer.Host, jwks.Host)
}

func canonicalClusterDiscoveryURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: base.Scheme, Host: base.Host}).String()
}

func canonicalClusterJWKSURL(baseURL, jwksURL string) string {
	if baseURL == "" || jwksURL == "" {
		return jwksURL
	}

	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return jwksURL
	}

	jwks, err := url.Parse(jwksURL)
	if err != nil || jwks.Scheme == "" || jwks.Host == "" {
		return jwksURL
	}

	if !isStandardKubernetesJWKSURL(jwksURL) {
		return jwksURL
	}

	return base.ResolveReference(&url.URL{
		Path:     jwks.Path,
		RawQuery: jwks.RawQuery,
	}).String()
}

func kubernetesJWKSFallbackURL(baseURL, jwksURL string) (string, bool) {
	if baseURL == "" || jwksURL == "" {
		return "", false
	}

	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", false
	}

	jwks, err := url.Parse(jwksURL)
	if err != nil || jwks.Scheme == "" || jwks.Host == "" {
		return "", false
	}

	if jwks.Host == base.Host && jwks.Scheme == base.Scheme {
		return "", false
	}

	if !isStandardKubernetesJWKSURL(jwksURL) {
		return "", false
	}

	return base.ResolveReference(&url.URL{
		Path:     jwks.Path,
		RawQuery: jwks.RawQuery,
	}).String(), true
}

func isStandardKubernetesJWKSURL(jwksURL string) bool {
	if jwksURL == "" {
		return false
	}
	jwks, err := url.Parse(jwksURL)
	if err != nil {
		return false
	}
	switch jwks.Path {
	case kubernetesOIDCJWKSPath, legacyOIDCJWKSPath:
		return true
	default:
		return false
	}
}

func pemPublicKeysFromJWKS(jwks jwksDocument) ([]string, error) {
	var pemKeys []string
	seen := make(map[string]struct{}, len(jwks.Keys))

	for _, key := range jwks.Keys {
		if len(key.X5c) > 0 {
			certDER, err := base64.StdEncoding.DecodeString(key.X5c[0])
			if err != nil {
				return nil, fmt.Errorf("failed to decode jwk x5c certificate: %w", err)
			}

			cert, err := x509.ParseCertificate(certDER)
			if err != nil {
				return nil, fmt.Errorf("failed to parse jwk x5c certificate: %w", err)
			}

			pubDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal jwk x5c public key: %w", err)
			}

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: publicKeyPEMBlockType, Bytes: pubDER}))
			if _, ok := seen[pemKey]; ok {
				continue
			}
			seen[pemKey] = struct{}{}
			pemKeys = append(pemKeys, pemKey)
			continue
		}

		switch key.Kty {
		case "RSA":
			nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
			if err != nil {
				return nil, fmt.Errorf("failed to decode rsa modulus: %w", err)
			}
			eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
			if err != nil {
				return nil, fmt.Errorf("failed to decode rsa exponent: %w", err)
			}
			if len(eBytes) == 0 {
				return nil, fmt.Errorf("rsa exponent is empty")
			}

			exponent := 0
			for _, b := range eBytes {
				exponent = exponent<<8 | int(b)
			}

			pubKey := &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: exponent,
			}

			pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal rsa public key: %w", err)
			}

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: publicKeyPEMBlockType, Bytes: pubDER}))
			if _, ok := seen[pemKey]; ok {
				continue
			}
			seen[pemKey] = struct{}{}
			pemKeys = append(pemKeys, pemKey)
		case "EC":
			var curve elliptic.Curve
			switch key.Crv {
			case "P-256":
				curve = elliptic.P256()
			case "P-384":
				curve = elliptic.P384()
			case "P-521":
				curve = elliptic.P521()
			default:
				return nil, fmt.Errorf("unsupported ec curve %q", key.Crv)
			}

			xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ec x coordinate: %w", err)
			}
			yBytes, err := base64.RawURLEncoding.DecodeString(key.Y)
			if err != nil {
				return nil, fmt.Errorf("failed to decode ec y coordinate: %w", err)
			}

			pubKey := &ecdsa.PublicKey{
				Curve: curve,
				X:     new(big.Int).SetBytes(xBytes),
				Y:     new(big.Int).SetBytes(yBytes),
			}

			pubDER, err := x509.MarshalPKIXPublicKey(pubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal ec public key: %w", err)
			}

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: publicKeyPEMBlockType, Bytes: pubDER}))
			if _, ok := seen[pemKey]; ok {
				continue
			}
			seen[pemKey] = struct{}{}
			pemKeys = append(pemKeys, pemKey)
		default:
			return nil, fmt.Errorf("unsupported jwk key type %q", key.Kty)
		}
	}

	if len(pemKeys) == 0 {
		return nil, fmt.Errorf("no public keys found in jwks")
	}

	return pemKeys, nil
}
