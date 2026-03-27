package openbaocluster

import (
	"context"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
)

func (r *OpenBaoClusterReconciler) setAPIServerNetworkReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	result := appopenbaocluster.EvaluateAPIServerNetwork(
		ctx,
		r.apiServerNetworkDependencies(),
		apiServerNetworkReasonPolicy(),
		cluster,
	)
	setAPIServerNetworkReadyEvaluatedCondition(cluster, result)
}
