package snapshot

import (
	"context"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
)

// EnsureRuntime bootstraps the shared backup runtime needed for pre-upgrade
// snapshot Jobs.
func EnsureRuntime(ctx context.Context, runtime portbackup.PreUpgradeSnapshotRuntime, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if runtime == nil {
		return fmt.Errorf("backup runtime is not configured")
	}
	if err := runtime.EnsureServiceAccount(ctx, cluster); err != nil {
		return fmt.Errorf("failed to ensure backup ServiceAccount: %w", err)
	}
	if err := runtime.EnsureRBAC(ctx, cluster); err != nil {
		return fmt.Errorf("failed to ensure backup RBAC: %w", err)
	}
	return nil
}
