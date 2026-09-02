package openbaocluster

import (
	"context"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// setTLSReadyCondition evaluates and sets the TLSReady condition.
func (r *OpenBaoClusterReconciler) setTLSReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	result := r.Applications.EvaluateTLSReadiness(ctx, cluster)
	setTLSReadyEvaluatedCondition(cluster, result)
}
