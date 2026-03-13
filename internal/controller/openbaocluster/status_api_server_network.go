package openbaocluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAPIServerNetworkReady),
		Status:             result.Status,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             result.Reason,
		Message:            result.Message,
	})
}
