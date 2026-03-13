package openbaocluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
)

func (r *OpenBaoClusterReconciler) setBackupConfigurationReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster.Spec.Backup == nil {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionBackupConfigurationReady))
		return
	}

	result, err := appopenbaocluster.EvaluateBackupConfiguration(ctx, r.Client, cluster)
	if err != nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionBackupConfigurationReady),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             reasonUnknown,
			Message:            fmt.Sprintf("Failed to evaluate backup Job prerequisites: %v", err),
		})
		return
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionBackupConfigurationReady),
		Status:             result.Status,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             result.Reason,
		Message:            result.Message,
	})
}
