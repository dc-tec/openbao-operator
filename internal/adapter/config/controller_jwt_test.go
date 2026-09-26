package config

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestRenderSelfInitHCL_ControllerTargetAudience(t *testing.T) {
	cluster := newMinimalCluster("bao", "tenant")
	cluster.UID = "cluster-uid"
	cluster.Spec.ControllerJWTMode = openbaov1alpha1.ControllerJWTModeTarget
	cluster.Spec.ReconcilePolicies = true
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true}}
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
	bootstrap := &OperatorBootstrapConfig{
		OIDCIssuerURL: "https://issuer", JWTKeysPEM: []string{"test-public-key"},
		OperatorNS: "operator", OperatorSA: "controller", JWTAuthAudience: "legacy-audience",
	}
	output, err := RenderSelfInitHCL(cluster, bootstrap)
	require.NoError(t, err)
	controller := selfInitRequestBlockForPath(t, string(output), pathAuthJWTRolePrefix+authRoleNameOperator)
	require.Contains(t, controller, `bound_audiences`)
	require.Contains(t, controller, `"urn:openbao:controller:cluster-uid"`)
	require.NotContains(t, controller, `"legacy-audience"`)
	require.Contains(t, controller, `"system:serviceaccount:operator:controller"`)
	require.Contains(t, controller, `"openbao-operator-policy-approval"`)
	for _, role := range []string{authRoleNameBackup, authRoleNameRestore, authRoleNameUpgrade} {
		job := selfInitRequestBlockForPath(t, string(output), pathAuthJWTRolePrefix+role)
		require.Contains(t, job, `"legacy-audience"`)
		require.NotContains(t, job, `urn:openbao:controller:`)
	}
	compareGolden(t, "render_self_init_target_controller", output)
	cluster.UID = ""
	_, err = RenderSelfInitHCL(cluster, bootstrap)
	require.ErrorContains(t, err, "UID is required")
}
