package upgrade

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/operationlock"
)

const UpgradeOperationLockHolder = constants.ControllerNameOpenBaoCluster + "/upgrade"

func IsOperationLockHeld(err error) bool {
	return errors.Is(err, operationlock.ErrLockHeld)
}

func IsUpgradeOperationLockHeldByUs(lock *openbaov1alpha1.OperationLockStatus) bool {
	return lock != nil &&
		lock.Operation == openbaov1alpha1.ClusterOperationUpgrade &&
		lock.Holder == UpgradeOperationLockHolder
}

func AcquireUpgradeOperationLock(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, message string) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return operationlock.Acquire(ctx, c, cluster, operationlock.AcquireOptions{
		Holder:    UpgradeOperationLockHolder,
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Message:   message,
	})
}

func ReleaseUpgradeOperationLock(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return operationlock.Release(ctx, c, cluster, UpgradeOperationLockHolder, openbaov1alpha1.ClusterOperationUpgrade)
}
