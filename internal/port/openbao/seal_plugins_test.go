package openbao

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestValidateSealPlugins(t *testing.T) {
	for _, sealType := range []string{SealTypeAWSKMS, SealTypeAzureKeyVault, SealTypeGCPCKMS, SealTypeOCIKMS, SealTypePKCS11} {
		t.Run(sealType, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: sealType}
			cluster.Spec.Version = "2.6.3"
			require.NoError(t, ValidateSealPlugins(cluster))
			for _, version := range []string{"2.7.0", "2.7.0-beta20260909", "v2.7.1", "3.0.0"} {
				cluster.Spec.Version = version
				require.ErrorContains(t, ValidateSealPlugins(cluster), "install the external KMS plugin before upgrading")
			}
			cluster.Spec.Plugins = []openbaov1alpha1.Plugin{{Type: "auth", Name: sealType, Command: "plugin"}}
			require.Error(t, ValidateSealPlugins(cluster))
			cluster.Spec.Plugins[0].Type = SealTypeKMSPlugin
			require.NoError(t, ValidateSealPlugins(cluster))
			cluster.Spec.Plugins[0].Command = ""
			require.Error(t, ValidateSealPlugins(cluster))
			cluster.Spec.Plugins[0].Image = "registry.example.com/kms:v1.0.0"
			require.NoError(t, ValidateSealPlugins(cluster))
		})
	}
	for _, sealType := range []string{SealTypeStatic, SealTypeTransit, SealTypeKMIP, SealTypeKMSPlugin} {
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		cluster.Spec.Version = "2.7.0"
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: sealType}
		require.NoError(t, ValidateSealPlugins(cluster))
	}
	require.NoError(t, ValidateSealPlugins(nil))
}
