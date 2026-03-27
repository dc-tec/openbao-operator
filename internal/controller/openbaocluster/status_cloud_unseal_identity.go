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
		setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to evaluate cloud KMS unseal identity prerequisites: %v", err),
		})
		return
	}

	setCloudUnsealIdentityReadyEvaluatedCondition(cluster, statusConditionResult{
		Status:  result.Status,
		Reason:  result.Reason,
		Message: result.Message,
	})
}
