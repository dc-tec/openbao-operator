package openbaocluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// setAuditFileStorageReadyCondition evaluates and sets AuditFileStorageReady when shared
// audit file storage is configured.
func (r *OpenBaoClusterReconciler) setAuditFileStorageReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	result, applicable := r.Applications.EvaluateAuditFileStorageReadiness(ctx, cluster)
	if !applicable {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
		return
	}
	setAuditFileStorageReadyEvaluatedCondition(cluster, result)
}
