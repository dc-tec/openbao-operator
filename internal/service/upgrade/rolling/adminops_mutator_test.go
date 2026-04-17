package rolling

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
)

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
