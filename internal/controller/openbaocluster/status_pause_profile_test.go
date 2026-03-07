package openbaocluster

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestUpdateStatusForPausedAndProfileNotSet(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	t.Run("paused cluster gets paused conditions", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

		if err := reconciler.updateStatusForPaused(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForPaused() error = %v", err)
		}
		if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseInitializing {
			t.Fatalf("phase = %s, want Initializing", cluster.Status.Phase)
		}
		for _, conditionType := range []openbaov1alpha1.ConditionType{
			openbaov1alpha1.ConditionAvailable,
			openbaov1alpha1.ConditionDegraded,
			openbaov1alpha1.ConditionTLSReady,
		} {
			cond := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
			if cond == nil {
				t.Fatalf("expected condition %s", conditionType)
			}
		}
	})

	t.Run("missing profile marks cluster blocked", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = ""
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

		if err := reconciler.updateStatusForProfileNotSet(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForProfileNotSet() error = %v", err)
		}
		if cluster.Status.Phase != openbaov1alpha1.ClusterPhaseInitializing {
			t.Fatalf("phase = %s, want Initializing", cluster.Status.Phase)
		}
		productionReady := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady))
		if productionReady == nil || productionReady.Reason != ReasonProfileNotSet {
			t.Fatalf("production-ready condition = %#v, want reason %q", productionReady, ReasonProfileNotSet)
		}
	})
}
