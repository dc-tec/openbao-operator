package openbaocluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func (r *OpenBaoClusterReconciler) setIngressIntegrationReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster.Spec.Ingress == nil || !cluster.Spec.Ingress.Enabled {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionIngressIntegrationReady))
		return
	}

	result := r.Applications.EvaluateIngressIntegration(ctx, cluster)
	setIngressIntegrationReadyEvaluatedCondition(cluster, result)
}
