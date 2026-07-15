package statusapply

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// OpenBaoClusterAdminOpsStatusApplyOptions configures adminops status SSA.
type OpenBaoClusterAdminOpsStatusApplyOptions struct {
	ForceOwnership bool
}

// ApplyOpenBaoClusterAdminOpsStatus applies the full adminops-owned status plane.
//
// All writers sharing the AdminOps field manager must apply the full plane, not
// disjoint subsets, otherwise later applies can clear fields omitted by earlier
// applies from the same manager.
func ApplyOpenBaoClusterAdminOpsStatus(
	ctx context.Context,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	opts OpenBaoClusterAdminOpsStatusApplyOptions,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}

	applyConfig, err := ToApplyConfiguration(&openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			AcceptedUpgradeStrategy: cluster.Status.AcceptedUpgradeStrategy,
			Upgrade:                 cluster.Status.Upgrade,
			UpgradeRequests:         cluster.Status.UpgradeRequests,
			Backup:                  cluster.Status.Backup,
			BlueGreen:               cluster.Status.BlueGreen,
			BreakGlass:              cluster.Status.BreakGlass,
			AdminOps:                cluster.Status.AdminOps,
		},
	}, c)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.SubResourceApplyOption{
		client.FieldOwner(constants.FieldOwnerAdminOpsStatus),
	}
	if opts.ForceOwnership {
		applyOpts = append(applyOpts, client.ForceOwnership)
	}

	return c.Status().Apply(ctx, applyConfig, applyOpts...)
}
