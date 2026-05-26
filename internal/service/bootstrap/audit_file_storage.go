package bootstrap

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func (m *Manager) ensureAuditFileStoragePVC(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if !portopenbao.UsesManagedAuditFileStorage(cluster) {
		return nil
	}

	pvc, err := buildManagedAuditFileStoragePVC(cluster)
	if err != nil {
		return err
	}

	if err := m.applyResourceWithoutOwnerRef(ctx, pvc); err != nil {
		return fmt.Errorf("failed to ensure audit file storage PVC %s/%s: %w", pvc.Namespace, pvc.Name, err)
	}

	logger.V(1).Info("Ensured managed audit file storage PVC", "pvc", pvc.Name)
	return nil
}

func buildManagedAuditFileStoragePVC(cluster *openbaov1alpha1.OpenBaoCluster) (*corev1.PersistentVolumeClaim, error) {
	if !portopenbao.UsesManagedAuditFileStorage(cluster) {
		return nil, fmt.Errorf("managed audit file storage PVC requested for cluster without managed audit file storage")
	}

	size, err := resource.ParseQuantity(cluster.Spec.AuditFileStorage.Size)
	if err != nil {
		return nil, fmt.Errorf("invalid audit file storage size %q: %w", cluster.Spec.AuditFileStorage.Size, err)
	}

	labels := resourceidentity.Labels(cluster)
	labels[constants.LabelOpenBaoAuditFileStorage] = "true"
	labels[constants.LabelOpenBaoSensitive] = constants.LabelValueSensitiveAudit

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      portopenbao.ManagedAuditFileStoragePVCName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}
	if sc := cluster.Spec.AuditFileStorage.StorageClassName; sc != nil && *sc != "" {
		className := *sc
		pvc.Spec.StorageClassName = &className
	}
	return pvc, nil
}
