package openbaocluster

import (
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestUpdateStatusRequiresApplications(t *testing.T) {
	reconciler := &OpenBaoClusterReconciler{}
	_, err := reconciler.updateStatus(t.Context(), logr.Discard(), newOpenBaoClusterStatusTestObject())
	if err == nil || !strings.Contains(err.Error(), "applications are not configured") {
		t.Fatalf("updateStatus() error = %v, want applications configuration error", err)
	}
}

func TestUpdateStatusReconcilesPrerequisiteConditions(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		Build()
	reconciler := &OpenBaoClusterReconciler{
		Client:       k8sClient,
		Applications: newStatusTestApplications(k8sClient, scheme),
	}

	if _, err := reconciler.updateStatus(t.Context(), logr.Discard(), cluster); err != nil {
		t.Fatalf("updateStatus() error = %v", err)
	}

	tlsReady := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
	if tlsReady == nil || tlsReady.Reason != "TLSSecretMissing" {
		t.Fatalf("TLSReady condition = %#v, want TLSSecretMissing", tlsReady)
	}
	if meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady)) == nil {
		t.Fatal("expected normal status policy to run after prerequisite reconciliation")
	}

	persisted := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(t.Context(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, persisted); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if persisted.Status.ObservedGeneration != cluster.Generation {
		t.Fatalf("persisted observedGeneration = %d, want %d", persisted.Status.ObservedGeneration, cluster.Generation)
	}
	if condition := meta.FindStatusCondition(persisted.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady)); condition == nil {
		t.Fatal("expected persisted TLSReady condition")
	}
}

func TestUpdateStatusForPaused(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		Build()
	reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

	if err := reconciler.updateStatusForPaused(t.Context(), logr.Discard(), cluster); err != nil {
		t.Fatalf("updateStatusForPaused() error = %v", err)
	}
	if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseInitializing {
		t.Fatalf("phase = %s, want Initializing", cluster.Status.Phase)
	}
	if cluster.Status.ObservedGeneration != cluster.Generation {
		t.Fatalf("observedGeneration = %d, want %d", cluster.Status.ObservedGeneration, cluster.Generation)
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
	if condition == nil || condition.Reason != "Paused" {
		t.Fatalf("Available condition = %#v, want Paused", condition)
	}
}

func TestUpdateStatusForProfileNotSet(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Spec.Profile = ""
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster.DeepCopy()).
		Build()
	reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

	if err := reconciler.updateStatusForProfileNotSet(t.Context(), logr.Discard(), cluster); err != nil {
		t.Fatalf("updateStatusForProfileNotSet() error = %v", err)
	}
	if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseInitializing {
		t.Fatalf("phase = %s, want Initializing", cluster.Status.Phase)
	}
	if cluster.Status.ObservedGeneration != cluster.Generation {
		t.Fatalf("observedGeneration = %d, want %d", cluster.Status.ObservedGeneration, cluster.Generation)
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady))
	if condition == nil || condition.Reason != ReasonProfileNotSet {
		t.Fatalf("ProductionReady condition = %#v, want %s", condition, ReasonProfileNotSet)
	}
}

func TestSafetyNetRequeueAfter(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 123456789)
	gotA := safetyNetRequeueAfter(now)
	gotB := safetyNetRequeueAfter(now)
	if gotA != gotB {
		t.Fatalf("expected deterministic output for the same timestamp: %v vs %v", gotA, gotB)
	}

	tests := []time.Time{
		time.Unix(1700000000, 0),
		time.Unix(1700000000, 1),
		time.Unix(1700000000, int64(constants.RequeueSafetyNetJitter/2)),
		time.Unix(1700000000, int64(constants.RequeueSafetyNetJitter-1)),
	}
	for _, ts := range tests {
		got := safetyNetRequeueAfter(ts)
		minRequeue := constants.RequeueSafetyNetBase
		maxRequeue := constants.RequeueSafetyNetBase + constants.RequeueSafetyNetJitter
		if got < minRequeue || got >= maxRequeue {
			t.Fatalf("safetyNetRequeueAfter(%v)=%v, expected in [%v, %v)", ts, got, minRequeue, maxRequeue)
		}
	}
}

func TestSteadyStateStatusRefreshRequeueAfter(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 123456789)
	got := steadyStateStatusRefreshRequeueAfter(now)
	if got != constants.RequeueStandard {
		t.Fatalf("steadyStateStatusRefreshRequeueAfter(%v)=%v, want %v", now, got, constants.RequeueStandard)
	}
}
