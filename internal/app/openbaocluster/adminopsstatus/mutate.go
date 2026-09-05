package adminopsstatus

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
	"github.com/dc-tec/openbao-operator/internal/port/adminops"
)

// NewMutator binds the shared AdminOps status writer to its Kubernetes reader
// and client. Use an uncached APIReader for immediate read-back visibility.
func NewMutator(reader client.Reader, c client.Client) adminops.StatusMutator {
	return func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, mutate func(*openbaov1alpha1.OpenBaoCluster) error, ownership adminops.OwnershipPolicy) error {
		return MutateWithReader(ctx, reader, c, cluster, mutate, ownership)
	}
}

// MutateWithReader applies an SSA read-modify-apply update for the
// adminops-owned status plane and syncs its fields and resource version from
// the read-back result. On error, cluster is unchanged, even if the apply
// succeeded and only the read-back failed.
func MutateWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	mutate func(obj *openbaov1alpha1.OpenBaoCluster) error,
	ownership adminops.OwnershipPolicy,
) error {
	if cluster == nil {
		return nil
	}
	if mutate == nil {
		return fmt.Errorf("mutate function is required")
	}
	var forceOwnership bool
	switch ownership {
	case adminops.RespectOwnership, adminops.ForceOwnershipOnConflict:
	case adminops.ForceOwnership:
		forceOwnership = true
	default:
		return fmt.Errorf("unsupported adminops ownership policy %d", ownership)
	}

	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	applyAdminOps := func(forceOwnership bool) (*openbaov1alpha1.OpenBaoCluster, error) {
		return statusapply.MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(ctx, reader, c, key, mutate, statusapply.OpenBaoClusterAdminOpsStatusApplyOptions{
			ForceOwnership: forceOwnership,
		})
	}

	updated, err := applyAdminOps(forceOwnership)
	if err != nil && apierrors.IsConflict(err) && ownership == adminops.ForceOwnershipOnConflict {
		updated, err = applyAdminOps(true)
	}
	if err != nil {
		return fmt.Errorf("failed to apply adminops status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	cluster.ResourceVersion = updated.ResourceVersion
	cluster.Status.AcceptedUpgradeStrategy = updated.Status.AcceptedUpgradeStrategy
	cluster.Status.Upgrade = updated.Status.Upgrade
	cluster.Status.UpgradeRequests = updated.Status.UpgradeRequests
	cluster.Status.Backup = updated.Status.Backup
	cluster.Status.Restore = updated.Status.Restore
	cluster.Status.BlueGreen = updated.Status.BlueGreen
	cluster.Status.BreakGlass = updated.Status.BreakGlass
	cluster.Status.AdminOps = updated.Status.AdminOps
	return nil
}
