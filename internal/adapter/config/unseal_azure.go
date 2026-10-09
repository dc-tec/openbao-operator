package config

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	platformsemver "github.com/dc-tec/openbao-operator/internal/platform/semver"
)

// validateAzureKeyVaultAuthMethod rejects spec.unseal.azureKeyVault.authMethod
// for OpenBao versions whose Azure Key Vault seal ignores auth_method. Before
// 2.6.0, the seal selects managed identity whenever a client ID is present
// without a client secret, so rendering the selector would not change the
// credential. The running version must support it too, so an in-progress
// upgrade never hands the selector to an older server.
func validateAzureKeyVaultAuthMethod(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Spec.Unseal == nil || cluster.Spec.Unseal.AzureKeyVault == nil ||
		cluster.Spec.Unseal.AzureKeyVault.AuthMethod == "" {
		return nil
	}
	versions := []string{cluster.Spec.Version}
	if cluster.Status.CurrentVersion != "" {
		versions = append(versions, cluster.Status.CurrentVersion)
	}
	for _, version := range versions {
		supported, err := platformsemver.AtLeast(version, 2, 6, 0)
		if err != nil {
			return fmt.Errorf("validate Azure Key Vault auth method version: %w", err)
		}
		if !supported {
			return fmt.Errorf("spec.unseal.azureKeyVault.authMethod requires OpenBao >= 2.6.0; complete the upgrade before configuring it (version=%q)", version)
		}
	}
	return nil
}
