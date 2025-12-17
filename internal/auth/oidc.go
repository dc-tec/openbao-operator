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
	"os"
	"time"

	"k8s.io/client-go/rest"
)

// OIDCConfig holds the discovered OIDC configuration.
type OIDCConfig struct {
	IssuerURL string
	CABundle  string
	JWKSKeys  []string
}

// DiscoverConfig fetches the Kubernetes OIDC issuer configuration at operator startup.
// baseURL allows tests (or specialized environments) to override the default
// Kubernetes API DNS name. When empty, it defaults to:
//
//	https://kubernetes.default.svc
//
// Returns an error if discovery fails. The operator can still run for Development
// profile clusters without OIDC, but Hardened profile requires OIDC.
func DiscoverConfig(ctx context.Context, cfg *rest.Config, baseURL string) (*OIDCConfig, error) {
	if baseURL == "" {
		baseURL = "https://kubernetes.default.svc"
	}
	wellKnownURL := baseURL + "/.well-known/openid-configuration"

	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC discovery request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC well-known endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC well-known endpoint returned status %d", resp.StatusCode)
	}

	var oidcConfig struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return nil, fmt.Errorf("failed to parse OIDC config: %w", err)
	}

	if oidcConfig.Issuer == "" {
		return nil, fmt.Errorf("OIDC config missing issuer")
	}

	issuerURL := oidcConfig.Issuer

	// Get CA bundle from REST config
	var caBundle string
	if len(cfg.CAData) > 0 {
		caBundle = string(cfg.CAData)
	} else if cfg.CAFile != "" {
		data, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caBundle = string(data)
	} else {
		// No CA configured - use system cert pool (may be empty)
		caBundle = ""
	}

	// Fetch JWKS keys if JWKS URI is available
	var jwksKeys []string
	if oidcConfig.JWKSURI != "" {
		keys, err := FetchJWKSKeys(ctx, cfg, oidcConfig.JWKSURI)
		if err != nil {
			// Log but don't fail - JWKS keys are optional for some configurations
			return &OIDCConfig{
				IssuerURL: issuerURL,
				CABundle:  caBundle,
				JWKSKeys:  nil,
			}, fmt.Errorf("failed to fetch JWKS keys: %w", err)
		}
		jwksKeys = keys
	}

	return &OIDCConfig{
		IssuerURL: issuerURL,
		CABundle:  caBundle,
		JWKSKeys:  jwksKeys,
	}, nil
}

type oidcDiscoveryDocument struct {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create jwks request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch jwks endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks endpoint returned status %d", resp.StatusCode)
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

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
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

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
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

			pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
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
