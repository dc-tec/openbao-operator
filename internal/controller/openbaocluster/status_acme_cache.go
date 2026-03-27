package openbaocluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// setACMECacheReadyCondition evaluates and sets the ACMECacheReady condition when a shared
// ACME cache is configured or required by the cluster topology.
func (r *OpenBaoClusterReconciler) setACMECacheReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	if !portopenbao.UsesACMEMode(cluster) || (!portopenbao.RequiresSharedACMECache(cluster) && !portopenbao.HasACMESharedCache(cluster)) {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady))
		return
	}

	claimName := portopenbao.ACMESharedCacheClaimName(cluster)
	if claimName == "" {
		setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  ReasonACMECacheNotConfigured,
			Message: "ACME shared cache is required for this topology; configure spec.tls.acme.sharedCache with a RWX PVC",
		})
		return
	}

	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: claimName}
	if err := r.Get(ctx, key, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  ReasonACMECacheMissing,
				Message: fmt.Sprintf("ACME shared cache PVC %s/%s was not found", cluster.Namespace, claimName),
			})
			return
		}
		setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to read ACME shared cache PVC %s/%s: %v", cluster.Namespace, claimName, err),
		})
		return
	}

	if !containsAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
		setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  ReasonACMECacheInvalidAccessMode,
			Message: fmt.Sprintf("ACME shared cache PVC %s/%s must support ReadWriteMany", pvc.Namespace, pvc.Name),
		})
		return
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  ReasonACMECachePending,
			Message: fmt.Sprintf("ACME shared cache PVC %s/%s is not Bound yet (phase=%s)", pvc.Namespace, pvc.Name, pvc.Status.Phase),
		})
		return
	}

	setACMECacheReadyEvaluatedCondition(cluster, statusConditionResult{
		Status:  metav1.ConditionTrue,
		Reason:  ReasonACMECacheReady,
		Message: fmt.Sprintf("ACME shared cache PVC %s/%s is Bound with ReadWriteMany access", pvc.Namespace, pvc.Name),
	})
}

func containsAccessMode(modes []corev1.PersistentVolumeAccessMode, want corev1.PersistentVolumeAccessMode) bool {
	for _, mode := range modes {
		if mode == want {
			return true
		}
	}
	return false
}
