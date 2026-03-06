package opslifecycle

import (
	"context"
	"errors"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/operationlock"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OperationLock describes the intended lock identity for a long-running operation.
type OperationLock struct {
	Holder    string
	Operation openbaov1alpha1.ClusterOperation
}

// AcquireOptions configures how lock acquisition should behave.
type AcquireOptions struct {
	Message string
	Force   bool
}

// IsHeldBy reports whether status lock matches the expected lock identity.
func (l OperationLock) IsHeldBy(lock *openbaov1alpha1.OperationLockStatus) bool {
	return lock != nil && lock.Operation == l.Operation && lock.Holder == l.Holder
}

// Acquire acquires or renews the lock on the cluster status.
func Acquire(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, lock OperationLock, opts AcquireOptions) error {
	return operationlock.Acquire(ctx, c, cluster, operationlock.AcquireOptions{
		Holder:    lock.Holder,
		Operation: lock.Operation,
		Message:   opts.Message,
		Force:     opts.Force,
	})
}

// Release releases the lock when it is owned by the given operation identity.
func Release(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, lock OperationLock) error {
	return operationlock.Release(ctx, c, cluster, lock.Holder, lock.Operation)
}

// IsLockHeld reports whether err indicates the lock is currently held by another operation.
func IsLockHeld(err error) bool {
	return errors.Is(err, operationlock.ErrLockHeld)
}

// HeldError extracts operation lock holder details from a contention error.
func HeldError(err error) (*operationlock.HeldError, bool) {
	var heldErr *operationlock.HeldError
	if !errors.As(err, &heldErr) {
		return nil, false
	}
	return heldErr, true
}

// AddHeldAuditFields enriches audit fields with contention details when available.
func AddHeldAuditFields(fields map[string]string, err error) {
	if fields == nil {
		return
	}
	heldErr, ok := HeldError(err)
	if !ok {
		return
	}
	fields["held_by_operation"] = string(heldErr.Operation)
	fields["held_by_holder"] = heldErr.Holder
}
