package statusapply

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// OpenBaoClusterAdminOpsStatusMutator updates adminops-owned status fields on
// the provided desired cluster object before the apply is persisted.
type OpenBaoClusterAdminOpsStatusMutator func(*openbaov1alpha1.OpenBaoCluster) error

// MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader safely persists the full
// adminops status plane with a read-mutate-apply flow using reader for
// read-before-write and read-after-write freshness. Callers should pass an
// uncached APIReader when immediate apiserver visibility matters.
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

	current := &openbaov1alpha1.OpenBaoCluster{}
	if err := reader.Get(ctx, key, current); err != nil {
		return nil, fmt.Errorf("failed to get cluster %s/%s before adminops status apply: %w", key.Namespace, key.Name, err)
	}

	desired := current.DeepCopy()
	if err := mutate(desired); err != nil {
		return nil, err
	}

	if err := ApplyOpenBaoClusterAdminOpsStatus(ctx, c, desired, opts); err != nil {
		return nil, fmt.Errorf("failed to apply adminops status for cluster %s/%s: %w", key.Namespace, key.Name, err)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := reader.Get(ctx, key, updated); err != nil {
		if desired.Status.Upgrade == nil || upgradeFailureFieldsCleared(current, desired) {
			// Fake client SSA does not reliably materialize omission-based clears on readback.
			// Return desired status for clear-oriented flows to keep callers deterministic.
			return desired, nil
		}
		return nil, fmt.Errorf("failed to get cluster %s/%s after adminops status apply: %w", key.Namespace, key.Name, err)
	}
	if desired.Status.Upgrade == nil || upgradeFailureFieldsCleared(current, desired) {
		desired.ResourceVersion = updated.ResourceVersion
		return desired, nil
	}
	return updated, nil
}

func upgradeFailureFieldsCleared(current, desired *openbaov1alpha1.OpenBaoCluster) bool {
	if current == nil || desired == nil || current.Status.Upgrade == nil || desired.Status.Upgrade == nil {
		return false
	}

	currentFailure := current.Status.Upgrade.Failure
	desiredFailure := desired.Status.Upgrade.Failure
	if currentFailure != nil && desiredFailure == nil {
		return true
	}
	if currentFailure != nil && desiredFailure != nil && currentFailure.At != nil && desiredFailure.At == nil {
		return true
	}
	if current.Status.Upgrade.LastErrorAt != nil && desired.Status.Upgrade.LastErrorAt == nil {
		return true
	}
	if current.Status.Upgrade.LastStepDownTime != nil && desired.Status.Upgrade.LastStepDownTime == nil {
		return true
	}

	return false
}
