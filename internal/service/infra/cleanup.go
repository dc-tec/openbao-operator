package infra

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// deletePVCs removes all PersistentVolumeClaims associated with the OpenBaoCluster.
func (m *Manager) deletePVCs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	var pvcList corev1.PersistentVolumeClaimList
	if err := m.client.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{constants.LabelOpenBaoCluster: cluster.Name}),
	); err != nil {
		return err
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if portopenbao.UsesExistingACMESharedCache(cluster) && pvc.Name == portopenbao.ACMESharedCacheClaimName(cluster) {
			continue
		}
		if len(pvc.Finalizers) > 0 {
			original := pvc.DeepCopy()
			pvc.Finalizers = nil
			if err := m.client.Patch(ctx, pvc, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		if err := m.client.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
