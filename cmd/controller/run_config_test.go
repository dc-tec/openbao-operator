package controller

import (
	"testing"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestBoolEnv(t *testing.T) {
	for _, tt := range []struct {
		name    string
		envKey  string
		load    func() (bool, error)
		errText string
	}{{
		name:    "service-claims",
		envKey:  constants.EnvOperatorEnableServiceClaims,
		load:    serviceClaimsEnabledFromEnv,
		errText: "serviceClaimsEnabledFromEnv()",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("defaults false when unset", func(t *testing.T) {
				t.Setenv(tt.envKey, "")

				got, err := tt.load()
				if err != nil {
					t.Fatalf("%s error = %v, want nil", tt.errText, err)
				}
				if got {
					t.Fatalf("%s = %v, want false", tt.errText, got)
				}
			})

			t.Run("parses true", func(t *testing.T) {
				t.Setenv(tt.envKey, "true")

				got, err := tt.load()
				if err != nil {
					t.Fatalf("%s error = %v, want nil", tt.errText, err)
				}
				if !got {
					t.Fatalf("%s = %v, want true", tt.errText, got)
				}
			})

			t.Run("returns error for invalid values", func(t *testing.T) {
				t.Setenv(tt.envKey, "definitely-not-bool")

				if _, err := tt.load(); err == nil {
					t.Fatalf("%s error = nil, want error", tt.errText)
				}
			})
		})
	}
}

func TestServiceClaimsTransitUnsealConfigFromEnv(t *testing.T) {
	t.Run("defaults empty when unset", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealAddress, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealKeyName, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealMountPath, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealNamespace, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealTLSServerName, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealCredentialsSecretName, "")

		got, err := serviceClaimsTransitUnsealConfigFromEnv()
		if err != nil {
			t.Fatalf("serviceClaimsTransitUnsealConfigFromEnv() error = %v, want nil", err)
		}
		if got != (serviceClaimsTransitUnsealEnvConfig{}) {
			t.Fatalf("serviceClaimsTransitUnsealConfigFromEnv() = %#v, want zero value", got)
		}
	})

	t.Run("parses complete config", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealAddress, "https://transit.example.internal:8200")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealKeyName, "openbao-unseal")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealMountPath, "transit")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealNamespace, "platform")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealTLSServerName, "transit.example.internal")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealCredentialsSecretName, "transit-unseal-creds")

		got, err := serviceClaimsTransitUnsealConfigFromEnv()
		if err != nil {
			t.Fatalf("serviceClaimsTransitUnsealConfigFromEnv() error = %v, want nil", err)
		}
		if got.address != "https://transit.example.internal:8200" ||
			got.keyName != "openbao-unseal" ||
			got.mountPath != "transit" ||
			got.namespace != "platform" ||
			got.tlsServerName != "transit.example.internal" ||
			got.credentialsSecretName != "transit-unseal-creds" {
			t.Fatalf("serviceClaimsTransitUnsealConfigFromEnv() = %#v, want parsed values", got)
		}
	})

	t.Run("rejects partial config", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealAddress, "https://transit.example.internal:8200")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealKeyName, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealMountPath, "transit")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealNamespace, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealTLSServerName, "")
		t.Setenv(constants.EnvOperatorServiceClaimsTransitUnsealCredentialsSecretName, "transit-unseal-creds")

		if _, err := serviceClaimsTransitUnsealConfigFromEnv(); err == nil {
			t.Fatal("serviceClaimsTransitUnsealConfigFromEnv() error = nil, want error")
		}
	})
}

func TestServiceClaimsNetworkConfigFromEnv(t *testing.T) {
	t.Run("defaults empty when unset", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerCIDR, "")
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerEndpointIPs, "")
		t.Setenv(constants.EnvOperatorServiceClaimsDNSEndpointIPs, "")

		got, err := serviceClaimsNetworkConfigFromEnv()
		if err != nil {
			t.Fatalf("serviceClaimsNetworkConfigFromEnv() error = %v, want nil", err)
		}
		if got.apiServerCIDR != "" || len(got.apiServerEndpointIPs) != 0 || len(got.dnsEndpointIPs) != 0 {
			t.Fatalf("serviceClaimsNetworkConfigFromEnv() = %#v, want zero value", got)
		}
	})

	t.Run("parses explicit cidr and endpoint ip lists", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerCIDR, "10.43.0.1/32")
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerEndpointIPs, " 172.29.0.2,172.29.0.2, 172.29.0.3 ")
		t.Setenv(constants.EnvOperatorServiceClaimsDNSEndpointIPs, "169.254.20.10")

		got, err := serviceClaimsNetworkConfigFromEnv()
		if err != nil {
			t.Fatalf("serviceClaimsNetworkConfigFromEnv() error = %v, want nil", err)
		}
		if got.apiServerCIDR != "10.43.0.1/32" {
			t.Fatalf("apiServerCIDR = %q, want 10.43.0.1/32", got.apiServerCIDR)
		}
		if len(got.apiServerEndpointIPs) != 2 ||
			got.apiServerEndpointIPs[0] != "172.29.0.2" ||
			got.apiServerEndpointIPs[1] != "172.29.0.3" {
			t.Fatalf("apiServerEndpointIPs = %v, want canonical deduplicated endpoint IPs", got.apiServerEndpointIPs)
		}
		if len(got.dnsEndpointIPs) != 1 || got.dnsEndpointIPs[0] != "169.254.20.10" {
			t.Fatalf("dnsEndpointIPs = %v, want 169.254.20.10", got.dnsEndpointIPs)
		}
	})

	t.Run("rejects invalid cidr", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerCIDR, "not-a-cidr")

		if _, err := serviceClaimsNetworkConfigFromEnv(); err == nil {
			t.Fatal("serviceClaimsNetworkConfigFromEnv() error = nil, want error")
		}
	})

	t.Run("rejects invalid endpoint ip", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerCIDR, "")
		t.Setenv(constants.EnvOperatorServiceClaimsAPIServerEndpointIPs, "172.29.0.2,definitely-not-ip")

		if _, err := serviceClaimsNetworkConfigFromEnv(); err == nil {
			t.Fatal("serviceClaimsNetworkConfigFromEnv() error = nil, want error")
		}
	})
}
