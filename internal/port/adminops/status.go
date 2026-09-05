// Package adminops defines the shared status persistence contract for
// administrative operations.
package adminops

import (
	"context"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// OwnershipPolicy controls when an AdminOps status write can take SSA field
// ownership. Conflict retries still use a fresh read under every policy.
type OwnershipPolicy uint8

const (
	// RespectOwnership never forces field ownership.
	RespectOwnership OwnershipPolicy = iota
	// ForceOwnershipOnConflict starts without force. If conflict retries are
	// exhausted, it repeats the read-mutate-apply flow with forced ownership.
	// This applies to both resource-version and field-ownership conflicts.
	ForceOwnershipOnConflict
	// ForceOwnership forces field ownership from the first apply attempt.
	ForceOwnership
)

// StatusMutator persists the full AdminOps status plane and syncs its fields
// and resource version on cluster from the read-back result. On error, cluster
// is unchanged, even if the apply succeeded and only the read-back failed.
// The callback can run more than once and must only mutate its supplied object.
type StatusMutator func(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
	mutate func(*openbaov1alpha1.OpenBaoCluster) error,
	ownership OwnershipPolicy,
) error
