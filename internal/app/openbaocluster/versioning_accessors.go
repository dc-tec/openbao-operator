package openbaocluster

import (
	"github.com/dc-tec/openbao-operator/internal/revision"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// OpenBaoClusterRevision computes the deterministic revision used by blue/green status logic.
func OpenBaoClusterRevision(version, image string, replicas int32) string {
	return revision.OpenBaoClusterRevision(version, image, replicas)
}

// IsVersionDowngrade reports whether moving from one version to another is a downgrade.
func IsVersionDowngrade(from, to string) (bool, error) {
	change, err := upgrade.CompareVersions(from, to)
	if err != nil {
		return false, err
	}
	return change == upgrade.VersionChangeDowngrade, nil
}
