package openbaocluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSetACMECacheReadyConditionAppliesApplicationResult(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default", Generation: 2},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeACME,
				ACME: &openbaov1alpha1.ACMEConfig{
					DirectoryURL: "https://acme.example/directory",
					SharedCache: &openbaov1alpha1.ACMESharedCacheConfig{
						Mode:              openbaov1alpha1.ACMESharedCacheModeExistingPVC,
						ExistingClaimName: "shared-cache",
					},
				},
			},
		},
	}
	statusReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-cache", Namespace: "default"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}).Build()
	reconciler := &OpenBaoClusterReconciler{
		Applications: newStatusTestApplications(statusReader, scheme),
	}

	reconciler.setACMECacheReadyCondition(t.Context(), cluster)

	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady))
	if condition == nil {
		t.Fatal("expected ACMECacheReady condition")
	}
	if condition.Status != metav1.ConditionTrue || condition.Reason != "ACMECacheReady" {
		t.Fatalf("ACMECacheReady condition = %#v, want status=True reason=ACMECacheReady", condition)
	}
	if condition.Message != "ACME shared cache PVC default/shared-cache is Bound with ReadWriteMany access" {
		t.Fatalf("message = %q", condition.Message)
	}
	if condition.ObservedGeneration != cluster.Generation {
		t.Fatalf("observed generation = %d, want %d", condition.ObservedGeneration, cluster.Generation)
	}
	if condition.LastTransitionTime.IsZero() {
		t.Fatal("expected nonzero last transition time")
	}
}

func TestSetACMECacheReadyConditionRemovesStaleConditionWhenNotApplicable(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Status.Conditions = []metav1.Condition{{
		Type:   string(openbaov1alpha1.ConditionACMECacheReady),
		Status: metav1.ConditionTrue,
		Reason: "ACMECacheReady",
	}}
	statusReader := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &OpenBaoClusterReconciler{
		Applications: newStatusTestApplications(statusReader, scheme),
	}

	reconciler.setACMECacheReadyCondition(t.Context(), cluster)

	if condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMECacheReady)); condition != nil {
		t.Fatalf("ACMECacheReady condition = %#v, want removed", condition)
	}
}
