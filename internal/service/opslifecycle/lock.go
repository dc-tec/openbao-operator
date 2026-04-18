package opslifecycle

import (
	"context"
	"errors"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

var (
	// ErrLockHeld indicates an operation lock is held by another operation/holder.
	ErrLockHeld = errors.New("operation lock is held by another operation")
)

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

// IsHeldBy reports whether status lock matches the expected lock identity.
func (l OperationLock) IsHeldBy(lock *openbaov1alpha1.OperationLockStatus) bool {
	return lock != nil && lock.Operation == l.Operation && lock.Holder == l.Holder
}

// Acquire acquires or renews the lock on the cluster status.
func Acquire(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, lock OperationLock, opts AcquireOptions) error {
	return AcquireWithReader(ctx, nil, c, cluster, lock, opts)
}

// AcquireWithReader acquires or renews the lock on the cluster status using a
// dedicated reader for fresh read-before-write visibility.
func AcquireWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	lock OperationLock,
	opts AcquireOptions,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if lock.Holder == "" {
		return fmt.Errorf("holder is required")
	}
	if lock.Operation == "" {
		return fmt.Errorf("operation is required")
	}

	now := metav1.Now()

	if cluster.Status.OperationLock == nil {
		desired := &openbaov1alpha1.OperationLockStatus{
			Operation:  lock.Operation,
			Holder:     lock.Holder,
			Message:    opts.Message,
			AcquiredAt: &now,
			RenewedAt:  &now,
		}
		if err := patchOperationLockStatus(ctx, reader, c, cluster, desired); err != nil {
			return err
		}
		cluster.Status.OperationLock = desired
		return nil
	}

	current := cluster.Status.OperationLock
	if current.Operation == lock.Operation && current.Holder == lock.Holder {
		desired := current.DeepCopy()
		desired.Message = opts.Message
		desired.RenewedAt = &now
		if desired.AcquiredAt == nil {
			desired.AcquiredAt = &now
		}
		if err := patchOperationLockStatus(ctx, reader, c, cluster, desired); err != nil {
			return err
		}
		cluster.Status.OperationLock = desired
		return nil
	}

	if opts.Force {
		desired := &openbaov1alpha1.OperationLockStatus{
			Operation:  lock.Operation,
			Holder:     lock.Holder,
			Message:    opts.Message,
			AcquiredAt: &now,
			RenewedAt:  &now,
		}
		if err := patchOperationLockStatus(ctx, reader, c, cluster, desired); err != nil {
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

// Release releases the lock when it is owned by the given operation identity.
func Release(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, lock OperationLock) error {
	return ReleaseWithReader(ctx, nil, c, cluster, lock)
}

// ReleaseWithReader releases the lock when it is owned by the given operation
// identity using a dedicated reader for fresh read-before-write visibility.
func ReleaseWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	lock OperationLock,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if lock.Holder == "" {
		return fmt.Errorf("holder is required")
	}
	if lock.Operation == "" {
		return fmt.Errorf("operation is required")
	}

	if cluster.Status.OperationLock == nil {
		return nil
	}

	current := cluster.Status.OperationLock
	if current.Operation != lock.Operation || current.Holder != lock.Holder {
		return &HeldError{
			Operation: current.Operation,
			Holder:    current.Holder,
			Message:   current.Message,
		}
	}

	if err := patchOperationLockStatus(ctx, reader, c, cluster, nil); err != nil {
		return err
	}
	cluster.Status.OperationLock = nil
	return nil
}

// IsLockHeld reports whether err indicates the lock is currently held by another operation.
func IsLockHeld(err error) bool {
	return errors.Is(err, ErrLockHeld)
}

// AsHeldError extracts operation lock holder details from a contention error.
func AsHeldError(err error) (*HeldError, bool) {
	var heldErr *HeldError
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
	heldErr, ok := AsHeldError(err)
	if !ok {
		return
	}
	fields["held_by_operation"] = string(heldErr.Operation)
	fields["held_by_holder"] = heldErr.Holder
}

func patchOperationLockStatus(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	desired *openbaov1alpha1.OperationLockStatus,
) error {
	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	applyLock := func(forceOwnership bool) (*openbaov1alpha1.OpenBaoCluster, error) {
		return statusapply.MutateAndApplyOpenBaoClusterOperationLockStatusWithReader(
			ctx,
			reader,
			c,
			key,
			func(obj *openbaov1alpha1.OpenBaoCluster) error {
				obj.Status.OperationLock = desired
				return nil
			},
			statusapply.OpenBaoClusterOperationLockStatusApplyOptions{
				ForceOwnership: forceOwnership,
			},
		)
	}

	updated, err := applyLock(false)
	if err != nil && apierrors.IsConflict(err) {
		updated, err = applyLock(true)
	}
	if err != nil {
		return fmt.Errorf("failed to apply operation lock status: %w", err)
	}

	cluster.ResourceVersion = updated.ResourceVersion
	return nil
}
