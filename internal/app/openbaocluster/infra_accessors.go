package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
)

// InfraCleanupDependencies are the minimal inputs required to run infra cleanup on deletion.
type InfraCleanupDependencies struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	OperatorNamespace string
	OIDCIssuer        string
	OIDCJWTKeys       []string
	Platform          string
}

// CleanupInfraOnDeletion performs infra cleanup for a deleting OpenBaoCluster.
func CleanupInfraOnDeletion(
	ctx context.Context,
	logger logr.Logger,
	deps InfraCleanupDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
	policy openbaov1alpha1.DeletionPolicy,
) error {
	infraMgr := inframanager.NewManagerWithReader(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		deps.OIDCIssuer,
		deps.OIDCJWTKeys,
		deps.Platform,
	)
	return infraMgr.Cleanup(ctx, logger, cluster, policy)
}

// BlueGreenActiveRevision returns the traffic-active revision used for selectors.
func BlueGreenActiveRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return inframanager.BlueGreenActiveRevision(cluster)
}
