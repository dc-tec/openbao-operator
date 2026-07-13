package openbaocluster

import (
	"context"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (r *OpenBaoClusterReconciler) setAPIServerNetworkReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	result := r.Applications.EvaluateAPIServerNetwork(ctx, cluster)
	setAPIServerNetworkReadyEvaluatedCondition(cluster, result)
}
