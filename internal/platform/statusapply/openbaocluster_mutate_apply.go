package statusapply

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// OpenBaoClusterAdminOpsStatusMutator updates adminops-owned status fields on
// the provided desired cluster object before the apply is persisted.
type OpenBaoClusterAdminOpsStatusMutator func(*openbaov1alpha1.OpenBaoCluster) error

// MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader safely persists the full
// adminops status plane with a read-mutate-apply flow using reader for
// read-before-write and read-after-write freshness. A resource-version conflict
// repeats the fresh read and mutation before another apply attempt. Callers
// should pass an uncached APIReader when immediate apiserver visibility matters.
// The result is the object read after the apply, which can include concurrent
// updates. A read-back failure returns an error even though the apply succeeded.
func MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	key types.NamespacedName,
	mutate OpenBaoClusterAdminOpsStatusMutator,
	opts OpenBaoClusterAdminOpsStatusApplyOptions,
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

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &openbaov1alpha1.OpenBaoCluster{}
		if err := reader.Get(ctx, key, current); err != nil {
			return fmt.Errorf("failed to get cluster %s/%s before adminops status apply: %w", key.Namespace, key.Name, err)
		}

		desired := current.DeepCopy()
		if err := mutate(desired); err != nil {
			return err
		}

		return ApplyOpenBaoClusterAdminOpsStatus(ctx, c, desired, opts)
	}); err != nil {
		return nil, fmt.Errorf("failed to apply adminops status for cluster %s/%s: %w", key.Namespace, key.Name, err)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := reader.Get(ctx, key, updated); err != nil {
		return nil, fmt.Errorf("failed to get cluster %s/%s after adminops status apply: %w", key.Namespace, key.Name, err)
	}
	return updated, nil
}
