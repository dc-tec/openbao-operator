package openbao

import (
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	platformsemver "github.com/dc-tec/openbao-operator/internal/platform/semver"
)

// ValidateSealPlugins rejects removed built-in seals before a workload or upgrade
// can replace a working Pod. Provider-specific configuration and credentials remain
// valid when a KMS plugin is declared under the same seal name.
func ValidateSealPlugins(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return nil
	}
	sealType := cluster.Spec.Unseal.Type
	switch sealType {
	case SealTypeAWSKMS, SealTypeAzureKeyVault, SealTypeGCPCKMS, SealTypeOCIKMS, SealTypePKCS11:
	default:
		return nil
	}
	version, err := platformsemver.Parse(cluster.Spec.Version)
	if err != nil {
		return fmt.Errorf("validate seal compatibility: %w", err)
	}
	if version.Major() < 2 || (version.Major() == 2 && version.Minor() < 7) {
		return nil
	}
	for _, plugin := range cluster.Spec.Plugins {
		if plugin.Type == SealTypeKMSPlugin && plugin.Name == sealType &&
			(strings.TrimSpace(plugin.Image) != "" || strings.TrimSpace(plugin.Command) != "") {
			return nil
		}
	}
	return fmt.Errorf("OpenBao %s requires a spec.plugins entry with type %q and name %q for spec.unseal.type=%q; "+
		"install the external KMS plugin before upgrading", cluster.Spec.Version, SealTypeKMSPlugin, sealType, sealType)
}
