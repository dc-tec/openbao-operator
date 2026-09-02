package openbaocluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSetAuditFileStorageReadyConditionAppliesApplicationResult(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default", Generation: 2},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			AuditFileStorage: &openbaov1alpha1.AuditFileStorageConfig{
				Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
				Size: "5Gi",
			},
		},
	}
	statusReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "example-audit", Namespace: "default"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}).Build()
	reconciler := &OpenBaoClusterReconciler{
		Applications: newStatusTestApplications(statusReader, scheme),
	}

	reconciler.setAuditFileStorageReadyCondition(t.Context(), cluster)

	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady))
	if condition == nil {
		t.Fatal("expected AuditFileStorageReady condition")
	}
	if condition.Status != metav1.ConditionTrue || condition.Reason != "AuditFileStorageReady" {
		t.Fatalf("AuditFileStorageReady condition = %#v, want status=True reason=AuditFileStorageReady", condition)
	}
	if condition.Message != "Audit file storage PVC default/example-audit is Bound with ReadWriteMany access" {
		t.Fatalf("message = %q", condition.Message)
	}
	if condition.ObservedGeneration != cluster.Generation {
		t.Fatalf("observed generation = %d, want %d", condition.ObservedGeneration, cluster.Generation)
	}
	if condition.LastTransitionTime.IsZero() {
		t.Fatal("expected nonzero last transition time")
	}
}

func TestSetAuditFileStorageReadyConditionRemovesStaleConditionWhenNotApplicable(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Status.Conditions = []metav1.Condition{{
		Type:   string(openbaov1alpha1.ConditionAuditFileStorageReady),
		Status: metav1.ConditionTrue,
		Reason: "AuditFileStorageReady",
	}}
	statusReader := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &OpenBaoClusterReconciler{
		Applications: newStatusTestApplications(statusReader, scheme),
	}

	reconciler.setAuditFileStorageReadyCondition(t.Context(), cluster)

	if condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAuditFileStorageReady)); condition != nil {
		t.Fatalf("AuditFileStorageReady condition = %#v, want removed", condition)
	}
}
