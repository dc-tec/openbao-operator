package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

// UpgradeOperationLockHolder is the shared holder identity for upgrade-owned
// operation locks.
const UpgradeOperationLockHolder = constants.ControllerNameOpenBaoCluster + "/upgrade"

var upgradeOperationLock = opslifecycle.OperationLock{
	Holder:    UpgradeOperationLockHolder,
	Operation: openbaov1alpha1.ClusterOperationUpgrade,
}

// LockAcquireResult captures whether acquisition was blocked by another
// operation while preserving the original held-lock error for caller-specific
// handling.
type LockAcquireResult struct {
	Blocked bool
	LockErr error
}

// IsOperationLockHeld reports whether the error indicates another operation is
// holding the cluster operation lock.
func IsOperationLockHeld(err error) bool {
	return opslifecycle.IsLockHeld(err)
}

// IsUpgradeOperationLockHeldByUs reports whether the current operation lock is
// owned by the upgrade flow.
func IsUpgradeOperationLockHeldByUs(lock *openbaov1alpha1.OperationLockStatus) bool {
	return upgradeOperationLock.IsHeldBy(lock)
}

// AcquireUpgradeOperationLockWithReader acquires the low-level upgrade
// operation lock using reader for fresh read-before-write visibility.
func AcquireUpgradeOperationLockWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
	message string,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return opslifecycle.AcquireWithReader(ctx, reader, c, cluster, upgradeOperationLock, opslifecycle.AcquireOptions{
		Message: message,
	})
}

// ReleaseUpgradeOperationLockWithReader releases the low-level upgrade
// operation lock using reader for fresh read-before-write visibility.
func ReleaseUpgradeOperationLockWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return opslifecycle.ReleaseWithReader(ctx, reader, c, cluster, upgradeOperationLock)
}

// AcquireUpgradeLockWithReader acquires the upgrade lock and emits the common
// audit event shape for success and lock contention using a dedicated reader.
func AcquireUpgradeLockWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	message string,
) (LockAcquireResult, error) {
	result := LockAcquireResult{}
	lockHeldByUs := false
	if cluster != nil {
		lockHeldByUs = IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock)
	}

	if err := AcquireUpgradeOperationLockWithReader(ctx, reader, c, cluster, message); err != nil {
		if !IsOperationLockHeld(err) {
			return result, err
		}

		result.Blocked = true
		result.LockErr = err

		fields := upgradeLockAuditFields(cluster)
		opslifecycle.AddHeldAuditFields(fields, err)
		logging.LogAuditEvent(logger, logging.EventOperationLockBlocked, fields)

		return result, nil
	}

	if !lockHeldByUs {
		logging.LogAuditEvent(logger, logging.EventOperationLockAcquired, upgradeLockAuditFields(cluster))
	}

	return result, nil
}

// ReleaseUpgradeLockIfHeldWithReader releases the upgrade lock when it is currently owned
// by the upgrade flow using a dedicated reader for fresh read-before-write visibility.
func ReleaseUpgradeLockIfHeldWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if !IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock) {
		return nil
	}

	if err := ReleaseUpgradeOperationLockWithReader(ctx, reader, c, cluster); err != nil {
		if IsOperationLockHeld(err) {
			logger.V(1).Info("Upgrade operation lock changed ownership before release")
			return nil
		}
		return fmt.Errorf("failed to release upgrade operation lock: %w", err)
	}

	logging.LogAuditEvent(logger, logging.EventOperationLockReleased, upgradeLockAuditFields(cluster))
	return nil
}

// ReleaseUpgradeLockOnErrorIfHeldWithReader joins the original cause with any
// upgrade lock release error when the caller indicates the upgrade never
// reached its active state, using a dedicated reader for fresh visibility.
func ReleaseUpgradeLockOnErrorIfHeldWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	shouldRelease bool,
	cause error,
	releaseErrorMessage string,
) error {
	if cause == nil || cluster == nil || !shouldRelease {
		return cause
	}

	if err := ReleaseUpgradeLockIfHeldWithReader(ctx, reader, c, logger, cluster); err != nil {
		if releaseErrorMessage == "" {
			return errors.Join(cause, err)
		}
		return errors.Join(cause, fmt.Errorf("%s: %w", releaseErrorMessage, err))
	}

	return cause
}

func upgradeLockAuditFields(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	if cluster == nil {
		return map[string]string{
			"operation": string(openbaov1alpha1.ClusterOperationUpgrade),
			"holder":    UpgradeOperationLockHolder,
		}
	}

	return map[string]string{
		"cluster_namespace": cluster.Namespace,
		"cluster_name":      cluster.Name,
		"operation":         string(openbaov1alpha1.ClusterOperationUpgrade),
		"holder":            UpgradeOperationLockHolder,
	}
}
