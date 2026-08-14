package statusapply

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
	if cluster.Status.OperationLock == nil {
		applyConfig, err = ToApplyConfigurationWithExplicitNulls(applyCluster, c, "status.operationLock")
	}
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

// MutateAndPatchOpenBaoClusterOperationLockStatusWithReader updates the lock
// plane with a fresh read and an optimistic status patch. A resource-version
// conflict repeats the fresh read and mutation before another patch attempt.
func MutateAndPatchOpenBaoClusterOperationLockStatusWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	key types.NamespacedName,
	mutate OpenBaoClusterOperationLockStatusMutator,
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
	if reader == nil {
		reader = c
	}

	var updated *openbaov1alpha1.OpenBaoCluster
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &openbaov1alpha1.OpenBaoCluster{}
		if err := reader.Get(ctx, key, current); err != nil {
			return fmt.Errorf(
				"failed to get cluster %s/%s before operation lock status patch: %w",
				key.Namespace,
				key.Name,
				err,
			)
		}

		desired := current.DeepCopy()
		if err := mutate(desired); err != nil {
			return err
		}
		if equality.Semantic.DeepEqual(current.Status.OperationLock, desired.Status.OperationLock) {
			updated = desired
			return nil
		}

		patched, err := patchOpenBaoClusterOperationLockStatus(ctx, c, current, desired.Status.OperationLock)
		if err != nil {
			return err
		}
		updated = patched
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to mutate operation lock status for cluster %s/%s: %w", key.Namespace, key.Name, err)
	}

	return updated, nil
}

func patchOpenBaoClusterOperationLockStatus(
	ctx context.Context,
	c client.Client,
	current *openbaov1alpha1.OpenBaoCluster,
	desiredLock *openbaov1alpha1.OperationLockStatus,
) (*openbaov1alpha1.OpenBaoCluster, error) {
	base := operationLockStatusPatchObject(current, current.Status.OperationLock)
	desired := operationLockStatusPatchObject(current, desiredLock)
	patch := client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})
	if err := c.Status().Patch(ctx, desired, patch); err != nil {
		return nil, err
	}
	return desired, nil
}

func operationLockStatusPatchObject(
	cluster *openbaov1alpha1.OpenBaoCluster,
	lock *openbaov1alpha1.OperationLockStatus,
) *openbaov1alpha1.OpenBaoCluster {
	var lockCopy *openbaov1alpha1.OperationLockStatus
	if lock != nil {
		lockCopy = lock.DeepCopy()
	}

	return &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            cluster.Name,
			Namespace:       cluster.Namespace,
			ResourceVersion: cluster.ResourceVersion,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			OperationLock: lockCopy,
		},
	}
}
