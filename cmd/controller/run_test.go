/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"k8s.io/client-go/rest"
)

func Test_pemPublicKeysFromJWKS_UsesX5C(t *testing.T) {
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

func Test_fetchOIDCJWKSKeys(t *testing.T) {
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

	jwksJSON := `{"keys":[{"kty":"RSA","x5c":["` + base64.StdEncoding.EncodeToString(certDER) + `"]}]}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openid/v1/jwks" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(jwksJSON))
	}))
	defer server.Close()

	cfg := &rest.Config{
		Host: server.URL,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	keys, err := fetchOIDCJWKSKeys(ctx, cfg, server.URL+"/openid/v1/jwks")
	if err != nil {
		t.Fatalf("fetchOIDCJWKSKeys() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("fetchOIDCJWKSKeys() keys = %d, want 1", len(keys))
	}
	if !strings.Contains(keys[0], "BEGIN PUBLIC KEY") {
		t.Fatalf("fetchOIDCJWKSKeys() key does not look like PEM public key, got:\n%s", keys[0])
	}
}

// Test_discoverOIDC tests the discoverOIDC function with various configurations.
func Test_discoverOIDC(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func(t *testing.T) *rest.Config
		wantErr     bool
		errContains string
		validateErr func(t *testing.T, err error)
	}{
		{
			name: "valid CAData - fails on HTTP request",
			setupConfig: func(t *testing.T) *rest.Config {
				// Create a test CA certificate
				caCert := &x509.Certificate{
					SerialNumber:          big.NewInt(1),
					Subject:               pkix.Name{CommonName: "test-ca"},
					NotBefore:             time.Now(),
					NotAfter:              time.Now().Add(24 * time.Hour),
					IsCA:                  true,
					BasicConstraintsValid: true,
				}
				caKey, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatalf("failed to generate CA key: %v", err)
				}
				caDER, err := x509.CreateCertificate(rand.Reader, caCert, caCert, &caKey.PublicKey, caKey)
				if err != nil {
					t.Fatalf("failed to create CA certificate: %v", err)
				}
				caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

				return &rest.Config{
					Host: "https://kubernetes.default.svc",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   caPEM,
						CAFile:   "",
						Insecure: false,
					},
				}
			},
			wantErr:     true,
			errContains: "failed to fetch OIDC well-known endpoint",
		},
		{
			name: "valid CAFile - fails on HTTP request",
			setupConfig: func(t *testing.T) *rest.Config {
				// Create a temporary CA file with valid PEM content
				tmpDir := t.TempDir()
				caFile := filepath.Join(tmpDir, "ca.crt")
				// Create a minimal valid PEM certificate
				caCert := &x509.Certificate{
					SerialNumber:          big.NewInt(1),
					Subject:               pkix.Name{CommonName: "test-ca"},
					NotBefore:             time.Now(),
					NotAfter:              time.Now().Add(24 * time.Hour),
					IsCA:                  true,
					BasicConstraintsValid: true,
				}
				caKey, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatalf("failed to generate CA key: %v", err)
				}
				caDER, err := x509.CreateCertificate(rand.Reader, caCert, caCert, &caKey.PublicKey, caKey)
				if err != nil {
					t.Fatalf("failed to create CA certificate: %v", err)
				}
				caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
				if err := os.WriteFile(caFile, caPEM, 0644); err != nil {
					t.Fatalf("failed to write CA file: %v", err)
				}

				return &rest.Config{
					Host: "https://kubernetes.default.svc",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   nil,
						CAFile:   caFile,
						Insecure: false,
					},
				}
			},
			wantErr:     true,
			errContains: "failed to fetch OIDC well-known endpoint",
		},
		{
			name: "no CA configured - fails on HTTP request",
			setupConfig: func(t *testing.T) *rest.Config {
				return &rest.Config{
					Host: "https://kubernetes.default.svc",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   nil,
						CAFile:   "",
						Insecure: false,
					},
				}
			},
			wantErr:     true,
			errContains: "failed to fetch OIDC well-known endpoint",
		},
		{
			name: "CAFile read error - non-existent file",
			setupConfig: func(t *testing.T) *rest.Config {
				// Use a non-existent file - this will fail during transport creation
				return &rest.Config{
					Host: "https://kubernetes.default.svc",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   nil,
						CAFile:   "/nonexistent/ca.crt",
						Insecure: false,
					},
				}
			},
			wantErr:     true,
			errContains: "failed to create transport", // Transport creation fails before HTTP request
			validateErr: func(t *testing.T, err error) {
				// Verify error mentions the file issue
				if !strings.Contains(err.Error(), "/nonexistent/ca.crt") &&
					!strings.Contains(err.Error(), "no such file") {
					t.Logf("Note: Error is about transport creation, not CA file read in discoverOIDC")
				}
			},
		},
		{
			name: "invalid CAData - invalid PEM",
			setupConfig: func(t *testing.T) *rest.Config {
				// Invalid PEM data
				invalidPEM := []byte("not a valid PEM certificate")
				return &rest.Config{
					Host: "https://kubernetes.default.svc",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   invalidPEM,
						CAFile:   "",
						Insecure: false,
					},
				}
			},
			wantErr:     true,
			errContains: "failed to create transport", // Transport creation fails with invalid PEM
		},
		{
			name: "empty host - still uses hardcoded URL",
			setupConfig: func(t *testing.T) *rest.Config {
				// Note: discoverOIDC uses a hardcoded URL, so Host in config doesn't affect it
				return &rest.Config{
					Host: "",
					TLSClientConfig: rest.TLSClientConfig{
						CAData:   nil,
						CAFile:   "",
						Insecure: false,
					},
				}
			},
			wantErr:     true,
			errContains: "failed to fetch OIDC well-known endpoint", // Still fails on HTTP request
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig(t)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			_, _, err := discoverOIDC(ctx, cfg, "")

			if (err != nil) != tt.wantErr {
				t.Errorf("discoverOIDC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				// Verify error message contains expected text
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("discoverOIDC() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				// Run custom validation if provided
				if tt.validateErr != nil {
					tt.validateErr(t, err)
				}
			}
		})
	}
}

// Test_discoverOIDC_SuccessWithCustomBaseURL verifies that discoverOIDC can
// successfully parse the issuer from a well-known endpoint when a custom base
// URL is provided. This uses an httptest server instead of a real Kubernetes
// API, and sets Insecure=true on the rest.Config to allow the self-signed
// certificate used by the test server.
func Test_discoverOIDC_SuccessWithCustomBaseURL(t *testing.T) {
	t.Helper()

	// Minimal OIDC discovery document.
	const issuer = "https://issuer.example"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"issuer":"` + issuer + `"}`))
	}))
	defer server.Close()

	cfg := &rest.Config{
		Host: server.URL,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	gotIssuer, caBundle, err := discoverOIDC(ctx, cfg, server.URL)
	if err != nil {
		t.Fatalf("discoverOIDC() unexpected error: %v", err)
	}
	if gotIssuer != issuer {
		t.Fatalf("discoverOIDC() issuer = %q, want %q", gotIssuer, issuer)
	}
	if caBundle != "" {
		t.Fatalf("discoverOIDC() caBundle = %q, want empty (no CAData/CAFile provided)", caBundle)
	}
}
