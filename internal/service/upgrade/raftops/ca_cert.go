package raftops

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
)

// LoadClusterCACert resolves the effective trust bundle for internal cluster
// clients. A nil CA bundle means the client should rely on system roots.
func LoadClusterCACert(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	return openbaotls.LoadClusterTrustBundle(ctx, c, cluster)
}
