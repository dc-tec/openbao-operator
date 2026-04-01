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

// AcquireUpgradeOperationLock acquires the low-level upgrade operation lock.
func AcquireUpgradeOperationLock(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, message string) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return opslifecycle.Acquire(ctx, c, cluster, upgradeOperationLock, opslifecycle.AcquireOptions{
		Message: message,
	})
}

// ReleaseUpgradeOperationLock releases the low-level upgrade operation lock.
func ReleaseUpgradeOperationLock(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return opslifecycle.Release(ctx, c, cluster, upgradeOperationLock)
}

// AcquireUpgradeLock acquires the upgrade lock and emits the common audit event
// shape for success and lock contention.
func AcquireUpgradeLock(ctx context.Context, c client.Client, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, message string) (LockAcquireResult, error) {
	result := LockAcquireResult{}
	lockHeldByUs := false
	if cluster != nil {
		lockHeldByUs = IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock)
	}

	if err := AcquireUpgradeOperationLock(ctx, c, cluster, message); err != nil {
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

// ReleaseUpgradeLockIfHeld releases the upgrade lock when it is currently owned
// by the upgrade flow. Ownership races are treated as benign.
func ReleaseUpgradeLockIfHeld(ctx context.Context, c client.Client, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if !IsUpgradeOperationLockHeldByUs(cluster.Status.OperationLock) {
		return nil
	}

	if err := ReleaseUpgradeOperationLock(ctx, c, cluster); err != nil {
		if IsOperationLockHeld(err) {
			logger.V(1).Info("Upgrade operation lock changed ownership before release")
			return nil
		}
		return fmt.Errorf("failed to release upgrade operation lock: %w", err)
	}

	logging.LogAuditEvent(logger, logging.EventOperationLockReleased, upgradeLockAuditFields(cluster))
	return nil
}

// ReleaseUpgradeLockOnErrorIfHeld joins the original cause with any upgrade
// lock release error when the caller indicates the upgrade never reached its
// active state.
func ReleaseUpgradeLockOnErrorIfHeld(
	ctx context.Context,
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

	if err := ReleaseUpgradeLockIfHeld(ctx, c, logger, cluster); err != nil {
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
