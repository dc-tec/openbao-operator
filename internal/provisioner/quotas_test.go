package provisioner

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGenerateTenantResourceQuota(t *testing.T) {
	namespace := "test-ns"

	tests := []struct {
		name string
		spec *corev1.ResourceQuotaSpec
		want *corev1.ResourceQuota
	}{
		{
			name: "Default Values",
			spec: nil,
			want: &corev1.ResourceQuota{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ResourceQuota",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      TenantResourceQuotaName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":       "openbao-operator",
						"app.kubernetes.io/component":  "provisioner",
						"app.kubernetes.io/managed-by": "openbao-operator",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						corev1.ResourcePods:           resource.MustParse("50"),
						corev1.ResourceRequestsCPU:    resource.MustParse("20"),
						corev1.ResourceRequestsMemory: resource.MustParse("64Gi"),
						corev1.ResourceLimitsCPU:      resource.MustParse("40"),
						corev1.ResourceLimitsMemory:   resource.MustParse("128Gi"),
					},
				},
			},
		},
		{
			name: "Custom Values",
			spec: &corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("10"),
				},
			},
			want: &corev1.ResourceQuota{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ResourceQuota",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      TenantResourceQuotaName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":       "openbao-operator",
						"app.kubernetes.io/component":  "provisioner",
						"app.kubernetes.io/managed-by": "openbao-operator",
					},
				},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						corev1.ResourcePods: resource.MustParse("10"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateTenantResourceQuota(namespace, tt.spec)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GenerateTenantResourceQuota() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateTenantLimitRange(t *testing.T) {
	namespace := "test-ns"

	tests := []struct {
		name string
		spec *corev1.LimitRangeSpec
		want *corev1.LimitRange
	}{
		{
			name: "Default Values",
			spec: nil,
			want: &corev1.LimitRange{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "LimitRange",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      TenantLimitRangeName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":       "openbao-operator",
						"app.kubernetes.io/component":  "provisioner",
						"app.kubernetes.io/managed-by": "openbao-operator",
					},
				},
				Spec: corev1.LimitRangeSpec{
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
				},
			},
		},
		{
			name: "Custom Values",
			spec: &corev1.LimitRangeSpec{
				Limits: []corev1.LimitRangeItem{
					{
						Type: corev1.LimitTypeContainer,
						Default: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			want: &corev1.LimitRange{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "LimitRange",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      TenantLimitRangeName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":       "openbao-operator",
						"app.kubernetes.io/component":  "provisioner",
						"app.kubernetes.io/managed-by": "openbao-operator",
					},
				},
				Spec: corev1.LimitRangeSpec{
					Limits: []corev1.LimitRangeItem{
						{
							Type: corev1.LimitTypeContainer,
							Default: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateTenantLimitRange(namespace, tt.spec)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GenerateTenantLimitRange() = %v, want %v", got, tt.want)
			}
		})
	}
}
