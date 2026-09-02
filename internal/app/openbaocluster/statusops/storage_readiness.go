package statusops

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// EvaluateACMECacheReadiness evaluates the shared ACME cache when it applies to the cluster.
// The reader is the reconciler's cached Kubernetes API reader.
func EvaluateACMECacheReadiness(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (ConditionResult, bool) {
	if !portopenbao.UsesACMEMode(cluster) ||
		(!portopenbao.RequiresSharedACMECache(cluster) && !portopenbao.HasACMESharedCache(cluster)) {
		return ConditionResult{}, false
	}

	claimName := portopenbao.ACMESharedCacheClaimName(cluster)
	if claimName == "" {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonACMECacheNotConfigured,
			Message: "ACME shared cache is required for this topology; configure spec.tls.acme.sharedCache with a RWX PVC",
		}, true
	}

	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: claimName}
	if err := reader.Get(ctx, key, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			return ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonACMECacheMissing,
				Message: fmt.Sprintf("ACME shared cache PVC %s/%s was not found", cluster.Namespace, claimName),
			}, true
		}
		return ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to read ACME shared cache PVC %s/%s: %v", cluster.Namespace, claimName, err),
		}, true
	}

	if !containsAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonACMECacheInvalidAccessMode,
			Message: fmt.Sprintf("ACME shared cache PVC %s/%s must support ReadWriteMany", pvc.Namespace, pvc.Name),
		}, true
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonACMECachePending,
			Message: fmt.Sprintf("ACME shared cache PVC %s/%s is not Bound yet (phase=%s)", pvc.Namespace, pvc.Name, pvc.Status.Phase),
		}, true
	}

	return ConditionResult{
		Status:  metav1.ConditionTrue,
		Reason:  reasonACMECacheReady,
		Message: fmt.Sprintf("ACME shared cache PVC %s/%s is Bound with ReadWriteMany access", pvc.Namespace, pvc.Name),
	}, true
}

// EvaluateAuditFileStorageReadiness evaluates shared audit file storage when it applies to the cluster.
// The reader is the reconciler's cached Kubernetes API reader.
func EvaluateAuditFileStorageReadiness(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (ConditionResult, bool) {
	if !portopenbao.HasAuditFileStorage(cluster) {
		return ConditionResult{}, false
	}

	claimName := portopenbao.AuditFileStorageClaimName(cluster)
	if claimName == "" {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonAuditFileStorageMissing,
			Message: "Audit file storage is configured but no PVC claim name could be resolved",
		}, true
	}

	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: claimName}
	if err := reader.Get(ctx, key, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			return ConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  reasonAuditFileStorageMissing,
				Message: fmt.Sprintf("Audit file storage PVC %s/%s was not found", cluster.Namespace, claimName),
			}, true
		}
		return ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to read audit file storage PVC %s/%s: %v", cluster.Namespace, claimName, err),
		}, true
	}

	if !containsAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonAuditFileStorageInvalidAccessMode,
			Message: fmt.Sprintf("Audit file storage PVC %s/%s must support ReadWriteMany", pvc.Namespace, pvc.Name),
		}, true
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		return ConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  reasonAuditFileStoragePending,
			Message: fmt.Sprintf("Audit file storage PVC %s/%s is not Bound yet (phase=%s)", pvc.Namespace, pvc.Name, pvc.Status.Phase),
		}, true
	}

	if stsName, recreateRequired, err := auditFileStorageStatefulSetRecreateRequired(ctx, reader, cluster); err != nil {
		return ConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to inspect OpenBao StatefulSets for audit file storage mounts: %v", err),
		}, true
	} else if recreateRequired {
		return ConditionResult{
			Status: metav1.ConditionFalse,
			Reason: constants.ReasonAuditFileStorageStatefulSetRecreateRequired,
			Message: fmt.Sprintf(
				"StatefulSet %s/%s is missing the requested audit file storage volume or mount; recreate the StatefulSet or create a new workload revision so locked pod-template fields can be applied",
				cluster.Namespace,
				stsName,
			),
		}, true
	}

	return ConditionResult{
		Status:  metav1.ConditionTrue,
		Reason:  reasonAuditFileStorageReady,
		Message: fmt.Sprintf("Audit file storage PVC %s/%s is Bound with ReadWriteMany access", pvc.Namespace, pvc.Name),
	}, true
}

func containsAccessMode(modes []corev1.PersistentVolumeAccessMode, want corev1.PersistentVolumeAccessMode) bool {
	for _, mode := range modes {
		if mode == want {
			return true
		}
	}
	return false
}

func auditFileStorageStatefulSetRecreateRequired(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (string, bool, error) {
	if reader == nil {
		return "", false, nil
	}

	var list appsv1.StatefulSetList
	if err := reader.List(
		ctx,
		&list,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(resourceidentity.Labels(cluster)),
	); err != nil {
		return "", false, err
	}

	for i := range list.Items {
		sts := &list.Items[i]
		if !statefulSetHasAuditFileStorageMount(sts, cluster) {
			return sts.Name, true, nil
		}
	}
	return "", false, nil
}

func statefulSetHasAuditFileStorageMount(
	sts *appsv1.StatefulSet,
	cluster *openbaov1alpha1.OpenBaoCluster,
) bool {
	if sts == nil {
		return true
	}

	claimName := portopenbao.AuditFileStorageClaimName(cluster)
	hasVolume := false
	for _, volume := range sts.Spec.Template.Spec.Volumes {
		if volume.Name == constants.VolumeAuditFileStorage &&
			volume.PersistentVolumeClaim != nil &&
			volume.PersistentVolumeClaim.ClaimName == claimName {
			hasVolume = true
			break
		}
	}
	if !hasVolume {
		return false
	}

	for _, container := range sts.Spec.Template.Spec.Containers {
		if container.Name != constants.ContainerBao {
			continue
		}
		for _, mount := range container.VolumeMounts {
			if mount.Name == constants.VolumeAuditFileStorage &&
				mount.MountPath == portopenbao.AuditFileStorageMountPath(cluster) &&
				mount.SubPathExpr == portopenbao.AuditFileStoragePodSubPathExpr {
				return true
			}
		}
		return false
	}
	return false
}
