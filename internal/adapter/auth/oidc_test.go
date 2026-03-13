package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestPemPublicKeysFromJWKS_UsesX5C(t *testing.T) {
	t.Helper()

	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	jwks := jwksDocument{
		Keys: []jwkKey{
			{
				Kty: "RSA",
				X5c: []string{base64.StdEncoding.EncodeToString(certDER)},
			},
		},
	}

	keys, err := pemPublicKeysFromJWKS(jwks)
	if err != nil {
		t.Fatalf("pemPublicKeysFromJWKS() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("pemPublicKeysFromJWKS() keys = %d, want 1", len(keys))
	}
	if !strings.Contains(keys[0], "BEGIN PUBLIC KEY") {
		t.Fatalf("pemPublicKeysFromJWKS() key does not look like PEM public key, got:\n%s", keys[0])
	}
}

func TestFetchJWKSKeys(t *testing.T) {
	t.Helper()

	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	// Create a test server that returns JWKS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != legacyOIDCJWKSPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		jwks := jwksDocument{
			Keys: []jwkKey{
				{
					Kty: "RSA",
					X5c: []string{base64.StdEncoding.EncodeToString(certDER)},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(jwks); err != nil {
			t.Errorf("failed to encode jwks: %v", err)
		}
	}))
	defer server.Close()

	cfg := &rest.Config{
		Host: server.URL,
	}

	keys, err := FetchJWKSKeys(context.Background(), cfg, server.URL+legacyOIDCJWKSPath)
	if err != nil {
		t.Fatalf("FetchJWKSKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("FetchJWKSKeys() keys = %d, want 1", len(keys))
	}
	if !strings.Contains(keys[0], "BEGIN PUBLIC KEY") {
		t.Fatalf("FetchJWKSKeys() key does not look like PEM public key, got:\n%s", keys[0])
	}
}

func TestDiscoverConfig(t *testing.T) {
	t.Helper()

	tests := []struct {
		name       string
		issuer     string
		jwksURI    string
		wantErr    bool
		wantJWKS   bool
		baseURL    string
		statusCode int
	}{
		{
			name:       "successful discovery with JWKS",
			issuer:     "https://example.com/issuer",
			jwksURI:    "", // Will be set to the test server URL in the test
			wantErr:    false,
			wantJWKS:   true,
			statusCode: http.StatusOK,
		},
		{
			name:       "successful discovery without JWKS",
			issuer:     "https://example.com/issuer",
			jwksURI:    "",
			wantErr:    false,
			wantJWKS:   false,
			statusCode: http.StatusOK,
		},
		{
			name:       "discovery endpoint not found",
			issuer:     "",
			jwksURI:    "",
			wantErr:    true,
			wantJWKS:   false,
			statusCode: http.StatusNotFound,
		},
		{
			name:       "missing issuer in response",
			issuer:     "",
			jwksURI:    "",
			wantErr:    true,
			wantJWKS:   false,
			statusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create JWKS server if needed
			var jwksServer *httptest.Server
			if tt.wantJWKS {
				// Generate a test certificate for JWKS
				cert := &x509.Certificate{
					SerialNumber:          big.NewInt(1),
					Subject:               pkix.Name{CommonName: "test-ca"},
					NotBefore:             time.Now(),
					NotAfter:              time.Now().Add(24 * time.Hour),
					IsCA:                  true,
					BasicConstraintsValid: true,
				}
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatalf("failed to generate rsa key: %v", err)
				}
				certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
				if err != nil {
					t.Fatalf("failed to create certificate: %v", err)
				}

				jwksServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != legacyOIDCJWKSPath {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					jwks := jwksDocument{
						Keys: []jwkKey{
							{
								Kty: "RSA",
								X5c: []string{base64.StdEncoding.EncodeToString(certDER)},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(jwks); err != nil {
						t.Errorf("failed to encode jwks: %v", err)
					}
				}))
				defer jwksServer.Close()
			}

			// Create a test server for OIDC discovery
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != oidcWellKnownConfigurationPath {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				if tt.statusCode != http.StatusOK {
					w.WriteHeader(tt.statusCode)
					return
				}

				jwksURI := tt.jwksURI
				if jwksURI == "" && tt.wantJWKS && jwksServer != nil {
					jwksURI = jwksServer.URL + legacyOIDCJWKSPath
				}

				config := map[string]interface{}{
					"issuer": tt.issuer,
				}
				if jwksURI != "" {
					config["jwks_uri"] = jwksURI
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(config); err != nil {
					t.Errorf("failed to encode config: %v", err)
				}
			}))
			defer server.Close()

			baseURL := tt.baseURL
			if baseURL == "" {
				baseURL = server.URL
			}

			cfg := &rest.Config{
				Host: server.URL,
			}

			config, err := DiscoverConfig(context.Background(), cfg, baseURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DiscoverConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if config == nil {
					t.Fatal("DiscoverConfig() returned nil config")
				}
				if config.IssuerURL != tt.issuer {
					t.Errorf("DiscoverConfig() IssuerURL = %q, want %q", config.IssuerURL, tt.issuer)
				}
				if tt.wantJWKS && len(config.JWKSKeys) == 0 {
					t.Error("DiscoverConfig() expected JWKS keys but got none")
				}
				if !tt.wantJWKS && len(config.JWKSKeys) > 0 {
					t.Errorf("DiscoverConfig() got JWKS keys but didn't expect any: %d", len(config.JWKSKeys))
				}
			}
		})
	}
}

func TestDiscoverConfig_FallsBackToDiscoveryBaseForStandardJWKSPath(t *testing.T) {
	t.Helper()

	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}
	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcWellKnownConfigurationPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":   "https://issuer.example.internal",
				"jwks_uri": "https://unreachable.example.internal/openid/v1/jwks",
			})
		case kubernetesOIDCJWKSPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(jwksDocument{
				Keys: []jwkKey{
					{
						Kty: "RSA",
						X5c: []string{base64.StdEncoding.EncodeToString(certDER)},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &rest.Config{Host: server.URL}
	config, err := DiscoverConfig(context.Background(), cfg, server.URL)
	if err != nil {
		t.Fatalf("DiscoverConfig() error = %v, want nil", err)
	}
	if config == nil {
		t.Fatal("DiscoverConfig() returned nil config")
	}
	if config.IssuerURL != "https://issuer.example.internal" {
		t.Fatalf("DiscoverConfig() IssuerURL = %q, want %q", config.IssuerURL, "https://issuer.example.internal")
	}
	if len(config.JWKSKeys) == 0 {
		t.Fatal("DiscoverConfig() expected JWKS keys from fallback path, got none")
	}
}

func TestDiscoverConfig_DoesNotRewriteCustomJWKSPath(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcWellKnownConfigurationPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":   "https://issuer.example.internal",
				"jwks_uri": "https://unreachable.example.internal/custom/jwks",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &rest.Config{Host: server.URL}
	config, err := DiscoverConfig(context.Background(), cfg, server.URL)
	if err == nil {
		t.Fatal("DiscoverConfig() expected error for unreachable custom jwks_uri, got nil")
	}
	if config == nil {
		t.Fatal("DiscoverConfig() expected partial config on JWKS fetch error, got nil")
	}
	if config.IssuerURL != "https://issuer.example.internal" {
		t.Fatalf("DiscoverConfig() IssuerURL = %q, want %q", config.IssuerURL, "https://issuer.example.internal")
	}
	if len(config.JWKSKeys) != 0 {
		t.Fatalf("DiscoverConfig() JWKSKeys = %d, want 0", len(config.JWKSKeys))
	}
	if !strings.Contains(err.Error(), "failed to fetch JWKS keys") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPemPublicKeysFromJWKS_RSA(t *testing.T) {
	t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate rsa key: %v", err)
	}

	// Encode modulus and exponent
	nBytes := key.N.Bytes()
	eBytes := big.NewInt(int64(key.E)).Bytes()

	jwks := jwksDocument{
		Keys: []jwkKey{
			{
				Kty: "RSA",
				N:   base64.RawURLEncoding.EncodeToString(nBytes),
				E:   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	}

	keys, err := pemPublicKeysFromJWKS(jwks)
	if err != nil {
		t.Fatalf("pemPublicKeysFromJWKS() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("pemPublicKeysFromJWKS() keys = %d, want 1", len(keys))
	}
	if !strings.Contains(keys[0], "BEGIN PUBLIC KEY") {
		t.Fatalf("pemPublicKeysFromJWKS() key does not look like PEM public key, got:\n%s", keys[0])
	}
}

func TestPemPublicKeysFromJWKS_EmptyKeys(t *testing.T) {
	t.Helper()

	jwks := jwksDocument{
		Keys: []jwkKey{},
	}

	keys, err := pemPublicKeysFromJWKS(jwks)
	if err == nil {
		t.Fatal("pemPublicKeysFromJWKS() expected error for empty keys, got nil")
	}
	if len(keys) != 0 {
		t.Fatalf("pemPublicKeysFromJWKS() keys = %d, want 0", len(keys))
	}
}

func TestPemPublicKeysFromJWKS_UnsupportedKeyType(t *testing.T) {
	t.Helper()

	jwks := jwksDocument{
		Keys: []jwkKey{
			{
				Kty: "UNSUPPORTED",
			},
		},
	}

	keys, err := pemPublicKeysFromJWKS(jwks)
	if err == nil {
		t.Fatal("pemPublicKeysFromJWKS() expected error for unsupported key type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("pemPublicKeysFromJWKS() error message should mention unsupported, got: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("pemPublicKeysFromJWKS() keys = %d, want 0", len(keys))
	}
}
