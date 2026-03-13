package openbaocluster

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSetACMECacheReadyCondition(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

	newCluster := func() *openbaov1alpha1.OpenBaoCluster {
		return &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Replicas: 3,
				TLS: openbaov1alpha1.TLSConfig{
					Enabled: true,
					Mode:    openbaov1alpha1.TLSModeACME,
					ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
				},
			},
		}
	}

	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		objects    []any
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "required cache not configured",
			cluster:    newCluster(),
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonACMECacheNotConfigured,
		},
		{
			name: "configured cache missing pvc",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newCluster()
				cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
					Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
					Size: "1Gi",
				}
				return cluster
			}(),
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonACMECacheMissing,
		},
		{
			name: "configured cache pending pvc",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newCluster()
				cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
					Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
					Size: "1Gi",
				}
				return cluster
			}(),
			objects: []any{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: "example-acme-cache", Namespace: "default"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					},
					Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonACMECachePending,
		},
		{
			name: "configured cache invalid access mode",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newCluster()
				cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
					Mode:              openbaov1alpha1.ACMESharedCacheModeExistingPVC,
					ExistingClaimName: "shared-cache",
				}
				return cluster
			}(),
			objects: []any{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: "shared-cache", Namespace: "default"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					},
					Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonACMECacheInvalidAccessMode,
		},
		{
			name: "configured cache ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newCluster()
				cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
					Mode:              openbaov1alpha1.ACMESharedCacheModeExistingPVC,
					ExistingClaimName: "shared-cache",
				}
				return cluster
			}(),
			objects: []any{
				&corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{Name: "shared-cache", Namespace: "default"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					},
					Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonACMECacheReady,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				builder = builder.WithObjects(obj.(client.Object))
			}
			reconciler := &OpenBaoClusterReconciler{Client: builder.Build()}

			reconciler.setACMECacheReadyCondition(context.Background(), tt.cluster)

			cond := meta.FindStatusCondition(tt.cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady))
			if cond == nil {
				t.Fatal("expected ACMECacheReady condition")
			}
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("ACMECacheReady = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
			}
		})
	}
}
