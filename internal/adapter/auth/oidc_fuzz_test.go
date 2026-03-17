package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

var (
	validJWKSX5C string
	validRSAN    string
	validRSAE    string
	validECX     string
	validECY     string
)

func init() {
	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().UTC(),
		NotAfter:              time.Now().UTC().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	certDER, err := x509.CreateCertificate(rand.Reader, cert, cert, &rsaKey.PublicKey, rsaKey)
	if err != nil {
		panic(err)
	}
	validJWKSX5C = base64.StdEncoding.EncodeToString(certDER)
	validRSAN = base64.RawURLEncoding.EncodeToString(rsaKey.N.Bytes())
	validRSAE = base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaKey.E)).Bytes())

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	validECX = base64.RawURLEncoding.EncodeToString(ecKey.X.Bytes())
	validECY = base64.RawURLEncoding.EncodeToString(ecKey.Y.Bytes())
}

func FuzzPemPublicKeysFromJWKS(f *testing.F) {
	f.Add("RSA", "", validRSAN, validRSAE, "", "", "")
	f.Add("EC", "P-256", "", "", validECX, validECY, "")
	f.Add("RSA", "", "", "", "", "", validJWKSX5C)
	f.Add("BAD", "", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, kty, crv, n, e, x, y, x5c string) {
		doc := jwksDocument{
			Keys: []jwkKey{{
				Kty: kty,
				Crv: crv,
				N:   n,
				E:   e,
				X:   x,
				Y:   y,
				X5c: fuzzX5C(x5c),
			}},
		}

		keys, err := pemPublicKeysFromJWKS(doc)
		if err == nil {
			if len(keys) == 0 {
				t.Fatalf("expected at least one PEM key on success")
			}
			for _, key := range keys {
				if !strings.Contains(key, "BEGIN PUBLIC KEY") {
					t.Fatalf("unexpected PEM output %q", key)
				}
			}
		}
	})
}

func FuzzFetchJWKSKeys(f *testing.F) {
	f.Add(http.StatusOK, fmt.Sprintf(`{"keys":[{"kty":"RSA","x5c":["%s"]}]}`, validJWKSX5C))
	f.Add(http.StatusOK, `{"keys":[]}`)
	f.Add(http.StatusNotFound, `not found`)

	f.Fuzz(func(t *testing.T, statusCode int, body string) {
		statusCode = normalizeStatusCode(statusCode)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/jwks" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(body))
		}))
		defer server.Close()

		keys, err := FetchJWKSKeys(context.Background(), &rest.Config{Host: server.URL}, server.URL+"/jwks")
		if statusCode != http.StatusOK {
			if err == nil {
				t.Fatalf("expected non-200 status to fail")
			}
			return
		}
		if err == nil {
			if len(keys) == 0 {
				t.Fatalf("expected non-empty key list on success")
			}
			for _, key := range keys {
				if !strings.Contains(key, "BEGIN PUBLIC KEY") {
					t.Fatalf("unexpected PEM output %q", key)
				}
			}
		}
	})
}

func FuzzDiscoverConfig(f *testing.F) {
	f.Add(http.StatusOK, `{"issuer":"https://issuer.example","jwks_uri":"__JWKS__"}`, http.StatusOK, fmt.Sprintf(`{"keys":[{"kty":"RSA","x5c":["%s"]}]}`, validJWKSX5C))
	f.Add(http.StatusOK, `{"issuer":"https://issuer.example"}`, http.StatusOK, `{"keys":[]}`)
	f.Add(http.StatusOK, `{}`, http.StatusOK, `{"keys":[]}`)
	f.Add(http.StatusNotFound, `missing`, http.StatusOK, `{"keys":[]}`)

	f.Fuzz(func(t *testing.T, discoveryStatus int, discoveryBody string, jwksStatus int, jwksBody string) {
		discoveryStatus = normalizeStatusCode(discoveryStatus)
		jwksStatus = normalizeStatusCode(jwksStatus)

		jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/jwks" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(jwksStatus)
			_, _ = w.Write([]byte(jwksBody))
		}))
		defer jwksServer.Close()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/.well-known/openid-configuration" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(discoveryStatus)
			_, _ = w.Write([]byte(strings.ReplaceAll(discoveryBody, "__JWKS__", jwksServer.URL+"/jwks")))
		}))
		defer server.Close()

		cfg, err := DiscoverConfig(context.Background(), &rest.Config{Host: server.URL}, server.URL)
		if err == nil {
			if cfg == nil {
				t.Fatalf("expected non-nil config on success")
			}
			if strings.TrimSpace(cfg.IssuerURL) == "" {
				t.Fatalf("expected issuer URL on success")
			}
		} else if cfg != nil && strings.TrimSpace(cfg.IssuerURL) != "" {
			// Partial OIDC discovery with issuer but failed JWKS fetch is allowed.
			return
		}
	})
}

func fuzzX5C(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return []string{raw}
}

func normalizeStatusCode(value int) int {
	if value < 100 || value > 599 {
		return http.StatusOK
	}
	if value < 200 || (value >= 300 && value < 400) {
		return http.StatusNotFound
	}
	return value
}
