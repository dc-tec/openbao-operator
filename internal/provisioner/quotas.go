package provisioner

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dc-tec/openbao-operator/internal/constants"
)

const (
	TenantResourceQuotaName = "openbao-operator-tenant-quota"
	TenantLimitRangeName    = "openbao-operator-tenant-limits"
)

// EnsureTenantQuotas ensures that default ResourceQuota and LimitRange exist in the tenant namespace.
// This prevents resource exhaustion (DoS) by a single tenant.
func (m *Manager) EnsureTenantQuotas(ctx context.Context, namespace string, quotaSpec *corev1.ResourceQuotaSpec, limitRangeSpec *corev1.LimitRangeSpec) error {
	// Apply ResourceQuota
	quota := GenerateTenantResourceQuota(namespace, quotaSpec)
	m.logger.Info("Applying tenant ResourceQuota", "namespace", namespace, "name", TenantResourceQuotaName,
		"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
	if err := m.applyResource(ctx, quota); err != nil {
		return fmt.Errorf("failed to apply ResourceQuota %s/%s: %w", namespace, TenantResourceQuotaName, err)
	}

	// Apply LimitRange
	limitRange := GenerateTenantLimitRange(namespace, limitRangeSpec)
	m.logger.Info("Applying tenant LimitRange", "namespace", namespace, "name", TenantLimitRangeName,
		"impersonating", fmt.Sprintf("system:serviceaccount:%s:%s", m.delegateSA.Namespace, m.delegateSA.Name))
	if err := m.applyResource(ctx, limitRange); err != nil {
		return fmt.Errorf("failed to apply LimitRange %s/%s: %w", namespace, TenantLimitRangeName, err)
	}

	return nil
}

func GenerateTenantResourceQuota(namespace string, spec *corev1.ResourceQuotaSpec) *corev1.ResourceQuota {
	q := &corev1.ResourceQuota{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ResourceQuota",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantResourceQuotaName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: constants.LabelValueAppComponentProvisioner,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
	}

	if spec != nil {
		q.Spec = *spec
	} else {
		q.Spec = corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourcePods:           resource.MustParse("50"),
				corev1.ResourceRequestsCPU:    resource.MustParse("20"),
				corev1.ResourceRequestsMemory: resource.MustParse("64Gi"),
				corev1.ResourceLimitsCPU:      resource.MustParse("40"),
				corev1.ResourceLimitsMemory:   resource.MustParse("128Gi"),
			},
		}
	}

	return q
}

func GenerateTenantLimitRange(namespace string, spec *corev1.LimitRangeSpec) *corev1.LimitRange {
	lr := &corev1.LimitRange{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "LimitRange",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      TenantLimitRangeName,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      constants.LabelValueAppNameOpenBaoOperator,
				constants.LabelAppComponent: constants.LabelValueAppComponentProvisioner,
				constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
			},
		},
	}

	if spec != nil {
		lr.Spec = *spec
	} else {
		lr.Spec = corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		}
	}

	return lr
}
