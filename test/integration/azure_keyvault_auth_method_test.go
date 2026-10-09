//go:build integration

package integration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func azureKeyVaultUnseal(authMethod string) *openbaov1alpha1.UnsealConfig {
	return &openbaov1alpha1.UnsealConfig{
		Type: "azurekeyvault",
		AzureKeyVault: &openbaov1alpha1.AzureKeyVaultSealConfig{
			VaultName:  "vault",
			KeyName:    "key",
			ClientID:   "client-456",
			AuthMethod: authMethod,
		},
	}
}

func TestVAP_OpenBaoCluster_AzureKeyVaultAuthMethod(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)
	tests := []struct {
		name       string
		version    string
		authMethod string
		wantError  string
	}{
		{name: "2.6.0", version: "2.6.0", authMethod: "workload_identity"},
		{name: "leading v and metadata", version: "v2.6.3+build.1", authMethod: "workload_identity"},
		{name: "unset on 2.4.4", version: "2.4.4"},
		{name: "2.5.5", version: "2.5.5", authMethod: "workload_identity", wantError: "requires OpenBao >= 2.6.0"},
		{name: "2.4.4", version: "2.4.4", authMethod: "managed_identity", wantError: "requires OpenBao >= 2.6.0"},
		{name: "2.6 prerelease", version: "2.6.0-beta1", authMethod: "workload_identity",
			wantError: "requires OpenBao >= 2.6.0"},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalClusterObj(namespace, fmt.Sprintf("azure-auth-method-%d", index))
			cluster.Spec.Version = tt.version
			cluster.Spec.Unseal = azureKeyVaultUnseal(tt.authMethod)
			err := k8sClient.Create(ctx, cluster, client.DryRunAll)
			if tt.wantError != "" {
				requireAdmissionDenied(t, err)
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVAP_OpenBaoCluster_AzureKeyVaultAuthMethodUpgrade(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)
	cluster := newMinimalClusterObj(namespace, "azure-auth-method-upgrade")
	cluster.Spec.Version = "2.5.5"
	cluster.Spec.Unseal = azureKeyVaultUnseal("")
	require.NoError(t, k8sClient.Create(ctx, cluster))
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) { status.CurrentVersion = "2.5.5" })
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, cluster))

	cluster.Spec.Version = "2.6.3"
	cluster.Spec.Unseal.AzureKeyVault.AuthMethod = "workload_identity"
	err := k8sClient.Update(ctx, cluster, client.DryRunAll)
	requireAdmissionDenied(t, err)
	require.ErrorContains(t, err, "Complete the OpenBao >= 2.6.0 upgrade")

	cluster.Spec.Unseal.AzureKeyVault.AuthMethod = ""
	require.NoError(t, k8sClient.Update(ctx, cluster))
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) { status.CurrentVersion = "2.6.3" })
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cluster.Name}, cluster))
	cluster.Spec.Unseal.AzureKeyVault.AuthMethod = "workload_identity"
	require.NoError(t, k8sClient.Update(ctx, cluster))
}
