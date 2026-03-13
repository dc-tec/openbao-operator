package openbaocluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
)

func (r *OpenBaoClusterReconciler) setCloudUnsealIdentityReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	result, applicable, err := appopenbaocluster.EvaluateCloudUnsealIdentity(ctx, r.Client, cluster)
	if !applicable {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
		return
	}

	if err != nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             reasonUnknown,
			Message:            fmt.Sprintf("Failed to evaluate cloud KMS unseal identity prerequisites: %v", err),
		})
		return
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
		Status:             result.Status,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             result.Reason,
		Message:            result.Message,
	})
}
