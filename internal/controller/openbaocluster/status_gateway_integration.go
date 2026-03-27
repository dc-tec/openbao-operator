package openbaocluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
)

func (r *OpenBaoClusterReconciler) setGatewayIntegrationReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
		return
	}

	result := appopenbaocluster.EvaluateGatewayIntegration(
		ctx,
		r.gatewayIntegrationDependencies(),
		cluster,
	)
	setGatewayIntegrationReadyEvaluatedCondition(cluster, result)
}
