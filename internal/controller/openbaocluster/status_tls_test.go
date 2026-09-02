package openbaocluster

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestSetTLSReadyConditionAppliesApplicationResult(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &OpenBaoClusterReconciler{
		Client:       fakeClient,
		Applications: newStatusTestApplications(fakeClient, scheme),
	}

	reconciler.setTLSReadyCondition(t.Context(), cluster)

	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
	if condition == nil {
		t.Fatal("expected TLSReady condition")
	}
	if condition.Status != metav1.ConditionFalse || condition.Reason != "TLSSecretMissing" {
		t.Fatalf("TLSReady condition = %#v, want status=False reason=TLSSecretMissing", condition)
	}
	if condition.Message != "CA TLS Secret is not present yet" {
		t.Fatalf("message = %q, want %q", condition.Message, "CA TLS Secret is not present yet")
	}
	if condition.ObservedGeneration != cluster.Generation {
		t.Fatalf("observed generation = %d, want %d", condition.ObservedGeneration, cluster.Generation)
	}
	if condition.LastTransitionTime.IsZero() {
		t.Fatal("expected nonzero last transition time")
	}
}
