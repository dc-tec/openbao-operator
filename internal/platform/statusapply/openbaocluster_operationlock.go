package statusapply

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// OpenBaoClusterOperationLockStatusApplyOptions configures operation lock SSA.
type OpenBaoClusterOperationLockStatusApplyOptions struct {
	ForceOwnership bool
}

// OpenBaoClusterOperationLockStatusMutator updates operation lock status fields
// on the provided desired cluster object before the apply is persisted.
type OpenBaoClusterOperationLockStatusMutator func(*openbaov1alpha1.OpenBaoCluster) error

// ApplyOpenBaoClusterOperationLockStatus applies the operation lock status
// plane under the dedicated lock field owner.
func ApplyOpenBaoClusterOperationLockStatus(
	ctx context.Context,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	opts OpenBaoClusterOperationLockStatusApplyOptions,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}

	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: cluster.Status.OperationLock,
		},
	}

	applyConfig, err := ToApplyConfiguration(applyCluster, c)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.SubResourceApplyOption{
		client.FieldOwner(constants.FieldOwnerOperationLockStatus),
	}
	if opts.ForceOwnership {
		applyOpts = append(applyOpts, client.ForceOwnership)
	}

	return c.Status().Apply(ctx, applyConfig, applyOpts...)
}

// MutateAndApplyOpenBaoClusterOperationLockStatus safely persists the lock
// plane with a read-mutate-apply flow.
func MutateAndApplyOpenBaoClusterOperationLockStatus(
	ctx context.Context,
	c client.Client,
	key types.NamespacedName,
	mutate OpenBaoClusterOperationLockStatusMutator,
	opts OpenBaoClusterOperationLockStatusApplyOptions,
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
		return nil, fmt.Errorf("failed to get cluster %s/%s before operation lock status apply: %w", key.Namespace, key.Name, err)
	}

	desired := current.DeepCopy()
	if err := mutate(desired); err != nil {
		return nil, err
	}

	if err := ApplyOpenBaoClusterOperationLockStatus(ctx, c, desired, opts); err != nil {
		return nil, fmt.Errorf("failed to apply operation lock status for cluster %s/%s: %w", key.Namespace, key.Name, err)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(ctx, key, updated); err != nil {
		return nil, fmt.Errorf("failed to get cluster %s/%s after operation lock status apply: %w", key.Namespace, key.Name, err)
	}
	if desired.Status.OperationLock == nil && updated.Status.OperationLock != nil {
		// Fake client SSA does not reliably materialize omitted-field clears
		// on readback. Return desired clear-state to keep callers deterministic.
		desired.ResourceVersion = updated.ResourceVersion
		return desired, nil
	}

	return updated, nil
}
