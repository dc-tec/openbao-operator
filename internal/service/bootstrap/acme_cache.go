package bootstrap

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceapply"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func (m *Manager) ensureACMESharedCachePVC(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if !portopenbao.UsesManagedACMESharedCache(cluster) {
		return nil
	}

	pvc, err := buildManagedACMESharedCachePVC(cluster)
	if err != nil {
		return err
	}

	if err := m.applyRetainedResource(ctx, pvc, cluster); err != nil {
		return fmt.Errorf("failed to ensure ACME shared cache PVC %s/%s: %w", pvc.Namespace, pvc.Name, err)
	}

	logger.V(1).Info("Ensured managed ACME shared cache PVC", "pvc", pvc.Name)
	return nil
}

func buildManagedACMESharedCachePVC(cluster *openbaov1alpha1.OpenBaoCluster) (*corev1.PersistentVolumeClaim, error) {
	if !portopenbao.UsesManagedACMESharedCache(cluster) {
		return nil, fmt.Errorf("managed ACME shared cache PVC requested for cluster without managed shared cache")
	}

	size, err := resource.ParseQuantity(cluster.Spec.TLS.ACME.SharedCache.Size)
	if err != nil {
		return nil, fmt.Errorf("invalid ACME shared cache size %q: %w", cluster.Spec.TLS.ACME.SharedCache.Size, err)
	}

	labels := resourceidentity.Labels(cluster)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      portopenbao.ManagedACMESharedCachePVCName(cluster),
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
	if sc := cluster.Spec.TLS.ACME.SharedCache.StorageClassName; sc != nil && *sc != "" {
		className := *sc
		pvc.Spec.StorageClassName = &className
	}
	if err := resourceownership.SetOwnerUIDAnnotation(pvc, cluster); err != nil {
		return nil, err
	}
	return pvc, nil
}

func (m *Manager) applyRetainedResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return resourceapply.ApplyRetained(ctx, m.client, cluster, obj)
}
