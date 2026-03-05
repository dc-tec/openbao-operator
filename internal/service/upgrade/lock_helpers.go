package upgrade

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/opslifecycle"
)

const UpgradeOperationLockHolder = constants.ControllerNameOpenBaoCluster + "/upgrade"

var upgradeOperationLock = opslifecycle.OperationLock{
	Holder:    UpgradeOperationLockHolder,
	Operation: openbaov1alpha1.ClusterOperationUpgrade,
}

func IsOperationLockHeld(err error) bool {
	return opslifecycle.IsLockHeld(err)
}

func IsUpgradeOperationLockHeldByUs(lock *openbaov1alpha1.OperationLockStatus) bool {
	return upgradeOperationLock.IsHeldBy(lock)
}

func AcquireUpgradeOperationLock(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster, message string) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return opslifecycle.Acquire(ctx, c, cluster, upgradeOperationLock, opslifecycle.AcquireOptions{
		Message: message,
	})
}

func ReleaseUpgradeOperationLock(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	return opslifecycle.Release(ctx, c, cluster, upgradeOperationLock)
}
