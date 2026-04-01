package statusapply

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// OpenBaoClusterStatusMergeMutator mutates the desired cluster object before a
// merge-patch status write.
type OpenBaoClusterStatusMergeMutator func(*openbaov1alpha1.OpenBaoCluster) error

// PatchOpenBaoClusterStatusMerge refreshes the latest cluster, applies mutate to
// a desired copy, and persists the status delta via merge patch.
func PatchOpenBaoClusterStatusMerge(
	ctx context.Context,
	c client.Client,
	key types.NamespacedName,
	mutate OpenBaoClusterStatusMergeMutator,
) (*openbaov1alpha1.OpenBaoCluster, error) {
	if c == nil {
		return nil, fmt.Errorf("client is required")
	}
	if key.Name == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if mutate == nil {
		return nil, fmt.Errorf("mutate function is required")
	}

	current := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(ctx, key, current); err != nil {
		return nil, fmt.Errorf("failed to get cluster %s/%s before status merge patch: %w", key.Namespace, key.Name, err)
	}

	desired := current.DeepCopy()
	if err := mutate(desired); err != nil {
		return nil, err
	}

	if err := c.Status().Patch(ctx, desired, client.MergeFrom(current)); err != nil {
		return nil, fmt.Errorf("failed to merge patch cluster status %s/%s: %w", key.Namespace, key.Name, err)
	}

	return desired, nil
}

// FinalizeRootUpgradeStatusMerge persists the common root-upgrade terminal
// status shape after a successful upgrade completes.
func FinalizeRootUpgradeStatusMerge(
	ctx context.Context,
	c client.Client,
	key types.NamespacedName,
	targetVersion string,
) (*openbaov1alpha1.OpenBaoCluster, error) {
	return PatchOpenBaoClusterStatusMerge(ctx, c, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.Upgrade = nil
		obj.Status.CurrentVersion = targetVersion
		return nil
	})
}
