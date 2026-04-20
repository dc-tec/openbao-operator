package openbaocluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func desiredStorageSpec(cluster *openbaov1alpha1.OpenBaoCluster) (resource.Quantity, string, error) {
	desiredQty, err := resource.ParseQuantity(cluster.Spec.Storage.Size)
	if err != nil {
		return resource.Quantity{}, "", operatorerrors.WithReason(
			constants.ReasonStorageInvalidSize,
			operatorerrors.WrapPermanentConfig(fmt.Errorf("invalid spec.storage.size %q: %w", cluster.Spec.Storage.Size, err)),
		)
	}

	var desiredStorageClassName string
	if cluster.Spec.Storage.StorageClassName != nil && *cluster.Spec.Storage.StorageClassName != "" {
		desiredStorageClassName = *cluster.Spec.Storage.StorageClassName
	}

	return desiredQty, desiredStorageClassName, nil
}

func desiredReadReplicaStorageSpec(cluster *openbaov1alpha1.OpenBaoCluster, voterQty resource.Quantity, voterStorageClassName string) (resource.Quantity, string, bool, error) {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return resource.Quantity{}, "", false, nil
	}

	desiredQty := voterQty.DeepCopy()
	desiredStorageClassName := voterStorageClassName

	if cluster.Spec.ReadReplicas.Storage != nil {
		if cluster.Spec.ReadReplicas.Storage.Size != nil {
			desiredQty = cluster.Spec.ReadReplicas.Storage.Size.DeepCopy()
		}
		if cluster.Spec.ReadReplicas.Storage.StorageClassName != nil && *cluster.Spec.ReadReplicas.Storage.StorageClassName != "" {
			desiredStorageClassName = *cluster.Spec.ReadReplicas.Storage.StorageClassName
		}
	}

	if desiredQty.Cmp(voterQty) < 0 {
		return resource.Quantity{}, "", false, operatorerrors.WithReason(
			constants.ReasonStorageInvalidSize,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"spec.readReplicas.storage.size cannot be smaller than spec.storage.size (%s < %s)",
				desiredQty.String(), voterQty.String(),
			)),
		)
	}

	return desiredQty, desiredStorageClassName, true, nil
}

type clusterDataPVCs struct {
	Voters       []corev1.PersistentVolumeClaim
	ReadReplicas []corev1.PersistentVolumeClaim
}

func listClusterPVCs(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) (clusterDataPVCs, error) {
	var pvcList corev1.PersistentVolumeClaimList
	if err := c.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{labelOpenBaoCluster: cluster.Name}),
	); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return clusterDataPVCs{}, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err))
		}
		return clusterDataPVCs{}, fmt.Errorf("failed to list PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	dataPVCs := clusterDataPVCs{
		Voters:       make([]corev1.PersistentVolumeClaim, 0, len(pvcList.Items)),
		ReadReplicas: make([]corev1.PersistentVolumeClaim, 0, len(pvcList.Items)),
	}
	for i := range pvcList.Items {
		pvc := pvcList.Items[i]
		if !isManagedDataPVC(cluster.Name, pvc.Name) {
			continue
		}
		if isReadReplicaDataPVC(cluster, pvc.Name) {
			dataPVCs.ReadReplicas = append(dataPVCs.ReadReplicas, pvc)
			continue
		}
		dataPVCs.Voters = append(dataPVCs.Voters, pvc)
	}

	return dataPVCs, nil
}

func isManagedDataPVC(clusterName, pvcName string) bool {
	return strings.HasPrefix(pvcName, storageVolumeDataPrefix+clusterName+"-")
}

func isReadReplicaDataPVC(cluster *openbaov1alpha1.OpenBaoCluster, pvcName string) bool {
	return strings.HasPrefix(pvcName, storageVolumeDataPrefix+resourceidentity.ReadReplicaStatefulSetName(cluster)+"-")
}

func validateStorageChangeAllowed(fieldPath string, desiredQty resource.Quantity, desiredStorageClassName string, pvcs []corev1.PersistentVolumeClaim) error {
	for i := range pvcs {
		pvc := &pvcs[i]
		currentStorageClassName := ""
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
			currentStorageClassName = *pvc.Spec.StorageClassName
		}

		if desiredStorageClassName != "" && currentStorageClassName != desiredStorageClassName {
			return operatorerrors.WithReason(
				constants.ReasonStorageClassChangeNotSupported,
				operatorerrors.WrapPermanentConfig(fmt.Errorf(
					"%s.storageClassName cannot be changed for an existing cluster (PVC %s has %q, desired %q)",
					fieldPath, pvc.Name, currentStorageClassName, desiredStorageClassName,
				)),
			)
		}

		curr, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			continue
		}
		if desiredQty.Cmp(curr) < 0 {
			return operatorerrors.WithReason(
				constants.ReasonStorageShrinkNotSupported,
				operatorerrors.WrapPermanentConfig(fmt.Errorf(
					"%s.size cannot be decreased (requested %s but PVC %s already requests %s); revert the change",
					fieldPath, desiredQty.String(), pvc.Name, curr.String(),
				)),
			)
		}
	}

	return nil
}

func expandPVCs(
	ctx context.Context,
	c client.Client,
	recorder events.EventRecorder,
	cluster *openbaov1alpha1.OpenBaoCluster,
	logger logr.Logger,
	desiredQty resource.Quantity,
	pvcs []corev1.PersistentVolumeClaim,
) (int, error) {
	patched := 0
	for i := range pvcs {
		pvc := &pvcs[i]

		currentQty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if !ok {
			logger.V(1).Info("PVC missing storage request; skipping", "pvc", pvc.Name)
			continue
		}
		if desiredQty.Cmp(currentQty) <= 0 {
			continue
		}

		orig := pvc.DeepCopy()
		if pvc.Spec.Resources.Requests == nil {
			pvc.Spec.Resources.Requests = corev1.ResourceList{}
		}
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = desiredQty

		if err := c.Patch(ctx, pvc, client.MergeFrom(orig)); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
				return patched, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to patch PVC %s/%s for resize: %w", pvc.Namespace, pvc.Name, err))
			}
			if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
				return patched, operatorerrors.WithReason(
					constants.ReasonStorageResizeNotSupported,
					operatorerrors.WrapPermanentConfig(fmt.Errorf("PVC %s cannot be expanded to %s: %w", pvc.Name, desiredQty.String(), err)),
				)
			}
			return patched, fmt.Errorf("failed to patch PVC %s/%s for resize: %w", pvc.Namespace, pvc.Name, err)
		}

		patched++
		if recorder != nil {
			recorder.Eventf(cluster, nil, corev1.EventTypeNormal, eventReasonPVCResize, eventReasonPVCResize, "Resizing PVC %s from %s to %s", pvc.Name, currentQty.String(), desiredQty.String())
		}
	}

	return patched, nil
}
