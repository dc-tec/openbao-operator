package infra

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

func TestNewManagerWithReaderAndOIDCConfig(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	config := &portauth.OIDCConfig{
		IssuerURL:          "https://issuer.example",
		OIDCDiscoveryURL:   "https://issuer.example/.well-known/openid-configuration",
		OIDCDiscoveryCAPEM: "discovery-ca",
		JWKSURL:            "https://issuer.example/keys",
		JWKSCAPEM:          "jwks-ca",
		JWKSKeys:           []string{"key-1", "key-2"},
	}

	manager := NewManagerWithReaderAndOIDCConfig(
		k8sClient,
		nil,
		scheme,
		"openbao-operator-system",
		config,
		"test-platform",
	)

	if manager.reader != k8sClient {
		t.Fatalf("reader = %#v, want client fallback %#v", manager.reader, k8sClient)
	}
	if manager.oidcIssuer != config.IssuerURL {
		t.Fatalf("oidcIssuer = %q, want %q", manager.oidcIssuer, config.IssuerURL)
	}
	if manager.oidcDiscoveryURL != config.OIDCDiscoveryURL {
		t.Fatalf("oidcDiscoveryURL = %q, want %q", manager.oidcDiscoveryURL, config.OIDCDiscoveryURL)
	}
	if manager.oidcJWKSURL != config.JWKSURL {
		t.Fatalf("oidcJWKSURL = %q, want %q", manager.oidcJWKSURL, config.JWKSURL)
	}
	if len(manager.oidcJWTKeys) != len(config.JWKSKeys) {
		t.Fatalf("oidcJWTKeys len = %d, want %d", len(manager.oidcJWTKeys), len(config.JWKSKeys))
	}
	for i, key := range config.JWKSKeys {
		if manager.oidcJWTKeys[i] != key {
			t.Fatalf("oidcJWTKeys[%d] = %q, want %q", i, manager.oidcJWTKeys[i], key)
		}
	}
}
