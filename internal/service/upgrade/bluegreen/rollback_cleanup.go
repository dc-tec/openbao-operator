package bluegreen

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
)

// retireRollbackGreenData runs only after rollback repaired Blue consensus and
// removed Green membership. Removed Raft peers cannot rejoin using their old data.
// Keep the revision recorded until its workload and data claims are absent.
func (m *Manager) retireRollbackGreenData(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	status := cluster.Status.BlueGreen
	if status == nil || status.Phase != openbaov1alpha1.PhaseRollbackCleanup ||
		status.GreenRevision == "" || status.GreenRevision == status.BlueRevision {
		return false, fmt.Errorf("discarded Green revision is required for rollback cleanup")
	}
	if err := resourceownership.RequireOwnerUID(cluster); err != nil {
		return false, err
	}
	greenName := cluster.Name + "-" + status.GreenRevision
	if greenName == resourceidentity.ReadReplicaStatefulSetName(cluster) {
		return false, fmt.Errorf("rollback cleanup cannot retire the read-replica workload")
	}
	reader := m.reader
	if reader == nil {
		reader = m.client
	}
	sts := &appsv1.StatefulSet{}
	err := reader.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: greenName}, sts)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	if err == nil {
		if err := resourceownership.RequireManagedControllerOwnerProof("delete discarded Green StatefulSet", sts, cluster,
			openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster")); err != nil {
			return false, err
		}
		if sts.Spec.Template.Labels[constants.LabelOpenBaoRevision] != status.GreenRevision {
			return false, fmt.Errorf("StatefulSet %s does not identify the discarded Green revision", greenName)
		}
		if sts.DeletionTimestamp == nil {
			if err := m.client.Delete(ctx, sts, client.Preconditions{UID: &sts.UID, ResourceVersion: &sts.ResourceVersion}); client.IgnoreNotFound(err) != nil {
				return false, err
			}
		}
		return false, nil
	}

	// Include Terminating Pods and foreign Pods referencing these claims. A
	// deletion timestamp does not prove that the process has stopped using data.
	pods := &corev1.PodList{}
	if err := reader.List(ctx, pods, client.InNamespace(cluster.Namespace)); err != nil {
		return false, err
	}
	claimPrefix := constants.VolumeData + "-" + greenName + "-"
	for _, pod := range pods.Items {
		if rollbackOrdinalName(pod.Name, greenName+"-") {
			return false, nil
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && rollbackOrdinalName(volume.PersistentVolumeClaim.ClaimName, claimPrefix) {
				return false, nil
			}
		}
	}

	claims := &corev1.PersistentVolumeClaimList{}
	if err := reader.List(ctx, claims, client.InNamespace(cluster.Namespace)); err != nil {
		return false, err
	}
	var discarded []*corev1.PersistentVolumeClaim
	for i := range claims.Items {
		claim := &claims.Items[i]
		if !rollbackOrdinalName(claim.Name, claimPrefix) {
			continue
		}
		if !resourceownership.HasOwnerUIDAnnotation(claim, cluster) {
			return false, fmt.Errorf("discarded Green PVC %s lacks cluster UID ownership", claim.Name)
		}
		for key, value := range resourceidentity.Labels(cluster) {
			if claim.Labels[key] != value {
				return false, fmt.Errorf("discarded Green PVC %s lacks managed cluster labels", claim.Name)
			}
		}
		discarded = append(discarded, claim)
	}
	for _, claim := range discarded {
		if claim.DeletionTimestamp != nil {
			continue
		}
		if err := m.client.Delete(ctx, claim, client.Preconditions{UID: &claim.UID, ResourceVersion: &claim.ResourceVersion}); client.IgnoreNotFound(err) != nil {
			return false, err
		}
	}
	// Observe actual deletion on another reconcile; PVC protection and storage
	// finalizers remain in control of deletion completion.
	return len(discarded) == 0, nil
}

func rollbackOrdinalName(name, prefix string) bool {
	ordinal, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return false
	}
	value, err := strconv.ParseUint(ordinal, 10, 32)
	return err == nil && strconv.FormatUint(value, 10) == ordinal
}
