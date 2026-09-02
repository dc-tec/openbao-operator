package openbaocluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// setACMECacheReadyCondition evaluates and sets the ACMECacheReady condition when a shared
// ACME cache is configured or required by the cluster topology.
func (r *OpenBaoClusterReconciler) setACMECacheReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	result, applicable := r.Applications.EvaluateACMECacheReadiness(ctx, cluster)
	if !applicable {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady))
		return
	}
	setACMECacheReadyEvaluatedCondition(cluster, result)
}
