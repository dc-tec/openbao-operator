package rolling

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
)

// withoutUpgradeStatus leaves upgrade fields for seedUpgradeStatus to create
// through SSA. Fields seeded by fake.WithObjects have a different owner.
func withoutUpgradeStatus(cluster *openbaov1alpha1.OpenBaoCluster) *openbaov1alpha1.OpenBaoCluster {
	stored := cluster.DeepCopy()
	stored.Status.Upgrade = nil
	return stored
}

func seedUpgradeStatus(t *testing.T, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()
	progress := cluster.Status.Upgrade.DeepCopy()
	if err := adminopsstatus.MutateWithReader(context.Background(), c, c, cluster,
		func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.Upgrade = progress.DeepCopy()
			return nil
		}, adminopsstatus.MutateOptions{}); err != nil {
		t.Fatalf("seed upgrade status: %v", err)
	}
}

func testAdminOpsMutator(c client.Client) adminOpsStatusMutator {
	return func(
		ctx context.Context,
		cluster *openbaov1alpha1.OpenBaoCluster,
		mutate func(obj *openbaov1alpha1.OpenBaoCluster) error,
		forceOwnership bool,
	) error {
		return adminopsstatus.MutateWithReader(ctx, c, c, cluster, mutate, adminopsstatus.MutateOptions{
			ForceOwnership:  forceOwnership,
			RetryOnConflict: !forceOwnership,
		})
	}
}
