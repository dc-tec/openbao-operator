package revision

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const revisionLength = 16

// OpenBaoClusterRevision returns a deterministic revision string derived from fields
// that influence the StatefulSet identity for an OpenBaoCluster.
func OpenBaoClusterRevision(version, image string, replicas int32) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("version=%s\nimage=%s\nreplicas=%d\n", version, image, replicas)))
	return hex.EncodeToString(sum[:])[:revisionLength]
}
