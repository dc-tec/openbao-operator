package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	listenerTLSTestVersion       = "2.7.0"
	listenerTLSTestLegacyVersion = "2.6.3"
)

func TestRenderHCLListenerTLSPolicy(t *testing.T) {
	tests := []struct {
		name    string
		mode    openbaov1alpha1.TLSMode
		metrics bool
	}{
		{name: "operator managed"},
		{name: "external", mode: openbaov1alpha1.TLSModeExternal},
		{name: "ACME", mode: openbaov1alpha1.TLSModeACME},
		{name: "API and metrics", metrics: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalCluster("pq-tls", "default")
			cluster.Spec.Version = listenerTLSTestVersion
			cluster.Spec.TLS.Mode = tt.mode
			cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{Listener: hybridTLSPolicy()}
			if tt.mode == openbaov1alpha1.TLSModeACME {
				cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"}
			}
			listeners := 1
			if tt.metrics {
				listeners++
				cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{Metrics: &openbaov1alpha1.MetricsConfig{
					Enabled: true, ScrapeProfile: configScrapeProfileAllNodes,
				}}
			}
			got, err := RenderHCL(cluster, testInfrastructureDetails(cluster))
			require.NoError(t, err)
			for _, field := range []string{"tls_min_version", "tls_max_version", "tls_key_exchange_preferences"} {
				require.Equal(t, listeners, strings.Count(string(got), field), field)
			}
			if tt.metrics {
				compareGolden(t, "render_hcl_tls_hybrid_pq_metrics", got)
			}
		})
	}
}

func TestRenderHCLListenerTLSValidation(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*openbaov1alpha1.OpenBaoCluster)
		wantError string
	}{
		{name: "omitted defaults", configure: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.Configuration = nil }},
		{name: "hybrid enforcement"},
		{name: "mixed fallback", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSMinVersion = ""
			c.Spec.Configuration.Listener.TLSKeyExchangePreferences = append(
				c.Spec.Configuration.Listener.TLSKeyExchangePreferences, openbaov1alpha1.TLSKeyExchangeX25519)
		}},
		{name: "version bounds on older OpenBao", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Version = listenerTLSTestLegacyVersion
			c.Spec.Configuration.Listener.TLSKeyExchangePreferences = nil
		}},
		{name: "TLS disabled", configure: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.TLS.Enabled = false },
			wantError: "requires TLS enabled"},
		{name: "listener TLS disabled", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSDisable = ptr.To(true)
		}, wantError: "requires TLS enabled"},
		{name: "unsupported minimum", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSMinVersion = "tls11"
		}, wantError: "unsupported"},
		{name: "unsupported maximum", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSMaxVersion = "tls14"
		}, wantError: "unsupported"},
		{name: "reversed bounds", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSMaxVersion = openbaov1alpha1.TLSVersion12
		}, wantError: "greater than or equal"},
		{name: "empty groups", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSKeyExchangePreferences = []openbaov1alpha1.TLSKeyExchangeGroup{}
		}, wantError: "1 to 7"},
		{name: "duplicate group", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSKeyExchangePreferences = append(
				c.Spec.Configuration.Listener.TLSKeyExchangePreferences, openbaov1alpha1.TLSKeyExchangeX25519MLKEM768)
		}, wantError: "duplicate"},
		{name: "unknown group", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSKeyExchangePreferences[0] = "unknown"
		}, wantError: "unsupported group"},
		{name: "pure MLKEM unsupported", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSKeyExchangePreferences[0] = "MLKEM1024"
		}, wantError: "unsupported group"},
		{name: "PQ with default TLS minimum", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSMinVersion = ""
		}, wantError: "requires tlsMinVersion=tls13"},
		{name: "PQ with TLS12 minimum", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Spec.Configuration.Listener.TLSMinVersion = openbaov1alpha1.TLSVersion12
		}, wantError: "requires tlsMinVersion=tls13"},
		{name: "older OpenBao", configure: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.Version = listenerTLSTestLegacyVersion },
			wantError: "requires OpenBao >= 2.7.0"},
		{name: "2.7 prerelease", configure: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.Version = "2.7.0-beta1" },
			wantError: "requires OpenBao >= 2.7.0"},
		{name: "invalid version", configure: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.Version = "invalid" },
			wantError: "validate listener TLS key exchange version"},
		{name: "upgrade incomplete", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.CurrentVersion = listenerTLSTestLegacyVersion
		}, wantError: "complete the upgrade"},
		{name: "upgrade completed", configure: func(c *openbaov1alpha1.OpenBaoCluster) {
			c.Status.CurrentVersion = listenerTLSTestVersion
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalCluster("pq-tls", "default")
			cluster.Spec.Version = listenerTLSTestVersion
			cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{Listener: hybridTLSPolicy()}
			if tt.configure != nil {
				tt.configure(cluster)
			}
			got, err := RenderHCL(cluster, testInfrastructureDetails(cluster))
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			if cluster.Spec.Configuration == nil {
				require.NotContains(t, string(got), "tls_key_exchange_preferences")
				require.NotContains(t, string(got), "tls_min_version")
				require.NotContains(t, string(got), "tls_max_version")
			}
		})
	}
}

func hybridTLSPolicy() *openbaov1alpha1.ListenerConfig {
	return &openbaov1alpha1.ListenerConfig{
		TLSMinVersion: openbaov1alpha1.TLSVersion13,
		TLSMaxVersion: openbaov1alpha1.TLSVersion13,
		TLSKeyExchangePreferences: []openbaov1alpha1.TLSKeyExchangeGroup{
			openbaov1alpha1.TLSKeyExchangeX25519MLKEM768,
			openbaov1alpha1.TLSKeyExchangeSecP256r1MLKEM768,
			openbaov1alpha1.TLSKeyExchangeSecP384r1MLKEM1024,
		},
	}
}
