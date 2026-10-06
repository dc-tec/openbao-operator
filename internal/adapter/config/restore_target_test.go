package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestFreshRestoreConfigurationAvoidsDiscovery(t *testing.T) {
	cluster := newMinimalCluster("fresh", "recovery")
	cluster.Annotations = map[string]string{constants.AnnotationRestoreOrigin: "request"}
	cluster.Spec.SelfInit = &api.SelfInitConfig{Enabled: true}
	config, err := RenderHCL(cluster, testInfrastructureDetails(cluster))
	require.NoError(t, err)
	require.NotContains(t, string(config), "retry_join")
	require.NotContains(t, string(config), "service_registration")
	bootstrap := &OperatorBootstrapConfig{OperatorNS: "operator", OperatorSA: "controller",
		OIDCIssuerURL: "https://issuer.example", OIDCDiscoveryURL: "https://discovery.example"}
	_, err = RenderSelfInitHCL(cluster, bootstrap)
	require.ErrorContains(t, err, "requires observed JWT public keys")
	bootstrap.JWTKeysPEM = []string{"public-key"}
	config, err = RenderSelfInitHCL(cluster, bootstrap)
	require.NoError(t, err)
	require.Contains(t, string(config), "jwt_validation_pubkeys")
	require.NotContains(t, string(config), "oidc_discovery_url")
	require.NotContains(t, string(config), "jwks_url")
}
