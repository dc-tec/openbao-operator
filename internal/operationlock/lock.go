// Package operationlock provides a status-based mutual exclusion mechanism for
// long-running operations across controllers (upgrade/backup/restore).
//
// The lock is stored on OpenBaoCluster.Status.OperationLock and is intended to be:
// - Stable across controller restarts (Holder should be deterministic)
// - Strict by default (no automatic expiry)
// - Explicitly overridable only via an opt-in break-glass flag on the calling operation
package operationlock

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

var (
	// ErrLockHeld indicates an operation lock is held by another operation/holder.
	ErrLockHeld = errors.New("operation lock is held by another operation")
)

// AcquireOptions configures lock acquisition behavior.
type AcquireOptions struct {
	// Holder is a stable identifier for the lock holder (controller/component).
	Holder string
	// Operation is the operation requesting the lock.
	Operation openbaov1alpha1.ClusterOperation
	// Message provides human-readable context (optional).
	Message string
	// Force allows overwriting an existing lock. This should only be used for explicit
	// break-glass scenarios.
	Force bool
}

// HeldError provides structured information when a lock cannot be acquired.
type HeldError struct {
	Operation openbaov1alpha1.ClusterOperation
	Holder    string
	Message   string
}

func (e *HeldError) Error() string {
	return fmt.Sprintf("%s: operation=%q holder=%q message=%q", ErrLockHeld, e.Operation, e.Holder, e.Message)
}

func (e *HeldError) Unwrap() error {
	return ErrLockHeld
}

// Acquire ensures the given cluster holds the operation lock for opts.Operation.
// It patches the cluster status to persist the lock immediately.
func Acquire(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, opts AcquireOptions) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if opts.Holder == "" {
		return fmt.Errorf("holder is required")
	}
	if opts.Operation == "" {
		return fmt.Errorf("operation is required")
	}

	now := metav1.Now()

	if cluster.Status.OperationLock == nil {
		desired := &openbaov1alpha1.OperationLockStatus{
			Operation:  opts.Operation,
			Holder:     opts.Holder,
			Message:    opts.Message,
			AcquiredAt: &now,
			RenewedAt:  &now,
		}
		if err := patchStatus(ctx, c, cluster, desired); err != nil {
			return err
		}
		cluster.Status.OperationLock = desired
		return nil
	}

	current := cluster.Status.OperationLock
	if current.Operation == opts.Operation && current.Holder == opts.Holder {
		desired := current.DeepCopy()
		desired.Message = opts.Message
		desired.RenewedAt = &now
		if desired.AcquiredAt == nil {
			desired.AcquiredAt = &now
		}
		if err := patchStatus(ctx, c, cluster, desired); err != nil {
			return err
		}
		cluster.Status.OperationLock = desired
		return nil
	}

	if opts.Force {
		desired := &openbaov1alpha1.OperationLockStatus{
			Operation:  opts.Operation,
			Holder:     opts.Holder,
			Message:    opts.Message,
			AcquiredAt: &now,
			RenewedAt:  &now,
		}
		if err := patchStatus(ctx, c, cluster, desired); err != nil {
			return err
		}
		cluster.Status.OperationLock = desired
		return nil
	}

	return &HeldError{
		Operation: current.Operation,
		Holder:    current.Holder,
		Message:   current.Message,
	}
}

// Release clears the lock if it is held by opts.Operation/opts.Holder.
// If the lock is held by someone else, Release returns ErrLockHeld.
func Release(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, holder string, operation openbaov1alpha1.ClusterOperation) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if holder == "" {
		return fmt.Errorf("holder is required")
	}
	if operation == "" {
		return fmt.Errorf("operation is required")
	}

	if cluster.Status.OperationLock == nil {
		return nil
	}

	current := cluster.Status.OperationLock
	if current.Operation != operation || current.Holder != holder {
		return &HeldError{
			Operation: current.Operation,
			Holder:    current.Holder,
			Message:   current.Message,
		}
	}

	if err := patchStatus(ctx, c, cluster, nil); err != nil {
		return err
	}
	cluster.Status.OperationLock = nil
	return nil
}

func patchStatus(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, desired *openbaov1alpha1.OperationLockStatus) error {
	original := cluster.DeepCopy()
	toPatch := cluster.DeepCopy()
	toPatch.Status.OperationLock = desired

	if err := c.Status().Patch(ctx, toPatch, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to patch operation lock status: %w", err)
	}
	// Keep the caller object in sync with the apiserver's updated ResourceVersion
	// to avoid optimistic locking conflicts in later patches during the same reconcile.
	cluster.ObjectMeta.ResourceVersion = toPatch.ObjectMeta.ResourceVersion
	return nil
}
