package infra

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestBuildStatefulSetPodSecurityContext(t *testing.T) {
	tests := []struct {
		name     string
		cluster  *openbaov1alpha1.OpenBaoCluster
		platform string
		wantUser *int64
		wantGrp  *int64
		wantFS   *int64
	}{
		{
			name: "default kubernetes platform pins IDs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			platform: "kubernetes",
			wantUser: ptr.To(int64(constants.UserOpenBao)),
			wantGrp:  ptr.To(int64(constants.GroupOpenBao)),
			wantFS:   ptr.To(int64(constants.GroupOpenBao)),
		},
		{
			name: "empty platform defaults to pinning IDs (same as kubernetes)",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			platform: "",
			wantUser: ptr.To(int64(constants.UserOpenBao)),
			wantGrp:  ptr.To(int64(constants.GroupOpenBao)),
			wantFS:   ptr.To(int64(constants.GroupOpenBao)),
		},
		{
			name: "openshift platform omits IDs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{},
			},
			platform: "openshift",
			wantUser: nil,
			wantGrp:  nil,
			wantFS:   nil,
		},
		{
			name: "CRD override takes precedence over kubernetes default",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  ptr.To(int64(1001)),
						RunAsGroup: ptr.To(int64(2001)),
						FSGroup:    ptr.To(int64(3001)),
					},
				},
			},
			platform: "kubernetes",
			wantUser: ptr.To(int64(1001)),
			wantGrp:  ptr.To(int64(2001)),
			wantFS:   ptr.To(int64(3001)),
		},
		{
			name: "CRD override takes precedence over openshift default",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: ptr.To(int64(1001)),
					},
				},
			},
			platform: "openshift",
			wantUser: ptr.To(int64(1001)),
			wantGrp:  nil, // Should remain nil as not overridden
			wantFS:   nil, // Should remain nil as not overridden
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := buildStatefulSetPodSecurityContext(tt.cluster, tt.platform)

			if !ptrInt64Equal(sc.RunAsUser, tt.wantUser) {
				t.Errorf("RunAsUser = %v, want %v", ptrInt64Value(sc.RunAsUser), ptrInt64Value(tt.wantUser))
			}
			if !ptrInt64Equal(sc.RunAsGroup, tt.wantGrp) {
				t.Errorf("RunAsGroup = %v, want %v", ptrInt64Value(sc.RunAsGroup), ptrInt64Value(tt.wantGrp))
			}
			if !ptrInt64Equal(sc.FSGroup, tt.wantFS) {
				t.Errorf("FSGroup = %v, want %v", ptrInt64Value(sc.FSGroup), ptrInt64Value(tt.wantFS))
			}
		})
	}
}

func ptrInt64Equal(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrInt64Value(p *int64) int64 {
	if p == nil {
		return -1
	}
	return *p
}
