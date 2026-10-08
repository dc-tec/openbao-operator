package upgrade

import "github.com/kubebao/openbao-operator/internal/adapter/revision"

// OpenBaoClusterRevision returns a deterministic revision string for an OpenBaoCluster spec.
func OpenBaoClusterRevision(version, image string, replicas int32) string {
	return revision.OpenBaoClusterRevision(version, image, replicas)
}
