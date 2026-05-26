package openbaocluster

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// setAuditFileStorageReadyCondition evaluates and sets AuditFileStorageReady when shared
// audit file storage is configured.
func (r *OpenBaoClusterReconciler) setAuditFileStorageReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	if !portopenbao.HasAuditFileStorage(cluster) {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
		return
	}

	claimName := portopenbao.AuditFileStorageClaimName(cluster)
	if claimName == "" {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  ReasonAuditFileStorageMissing,
			Message: "Audit file storage is configured but no PVC claim name could be resolved",
		})
		return
	}

	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Namespace: cluster.Namespace, Name: claimName}
	if err := r.Get(ctx, key, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
				Status:  metav1.ConditionFalse,
				Reason:  ReasonAuditFileStorageMissing,
				Message: fmt.Sprintf("Audit file storage PVC %s/%s was not found", cluster.Namespace, claimName),
			})
			return
		}
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to read audit file storage PVC %s/%s: %v", cluster.Namespace, claimName, err),
		})
		return
	}

	if !containsAccessMode(pvc.Spec.AccessModes, corev1.ReadWriteMany) {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  ReasonAuditFileStorageInvalidAccessMode,
			Message: fmt.Sprintf("Audit file storage PVC %s/%s must support ReadWriteMany", pvc.Namespace, pvc.Name),
		})
		return
	}

	if pvc.Status.Phase != corev1.ClaimBound {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionFalse,
			Reason:  ReasonAuditFileStoragePending,
			Message: fmt.Sprintf("Audit file storage PVC %s/%s is not Bound yet (phase=%s)", pvc.Namespace, pvc.Name, pvc.Status.Phase),
		})
		return
	}

	if stsName, ok, err := auditFileStorageStatefulSetRecreateRequired(ctx, r.Client, cluster); err != nil {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status:  metav1.ConditionUnknown,
			Reason:  reasonUnknown,
			Message: fmt.Sprintf("Failed to inspect OpenBao StatefulSets for audit file storage mounts: %v", err),
		})
		return
	} else if ok {
		setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
			Status: metav1.ConditionFalse,
			Reason: ReasonAuditFileStorageStatefulSetRecreateRequired,
			Message: fmt.Sprintf(
				"StatefulSet %s/%s is missing the requested audit file storage volume or mount; recreate the StatefulSet or create a new workload revision so locked pod-template fields can be applied",
				cluster.Namespace,
				stsName,
			),
		})
		return
	}

	setAuditFileStorageReadyEvaluatedCondition(cluster, statusConditionResult{
		Status:  metav1.ConditionTrue,
		Reason:  ReasonAuditFileStorageReady,
		Message: fmt.Sprintf("Audit file storage PVC %s/%s is Bound with ReadWriteMany access", pvc.Namespace, pvc.Name),
	})
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

func statefulSetHasAuditFileStorageMount(sts *appsv1.StatefulSet, cluster *openbaov1alpha1.OpenBaoCluster) bool {
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
