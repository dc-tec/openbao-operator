package openbaocluster

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestUpdateStatusForPaused(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	t.Run("paused cluster gets paused conditions", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: "awskms",
			AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
				Region:   "eu-central-1",
				KMSKeyID: "alias/openbao",
			},
		}
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
		if cluster.Status.ObservedGeneration != cluster.Generation {
			t.Fatalf("observedGeneration = %d, want %d", cluster.Status.ObservedGeneration, cluster.Generation)
		}
		for _, conditionType := range []openbaov1alpha1.ConditionType{
			openbaov1alpha1.ConditionAvailable,
			openbaov1alpha1.ConditionDegraded,
			openbaov1alpha1.ConditionAPIServerNetworkReady,
			openbaov1alpha1.ConditionTLSReady,
			openbaov1alpha1.ConditionCloudUnsealIdentityReady,
			openbaov1alpha1.ConditionUserAccessBootstrap,
		} {
			cond := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
			if cond == nil {
				t.Fatalf("expected condition %s", conditionType)
			}
		}
	})

	t.Run("paused acme cluster gets paused acme integration condition", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
		cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"}
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

		if err := reconciler.updateStatusForPaused(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForPaused() error = %v", err)
		}
		if cluster.Status.ObservedGeneration != cluster.Generation {
			t.Fatalf("observedGeneration = %d, want %d", cluster.Status.ObservedGeneration, cluster.Generation)
		}
		cond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
		if cond == nil || cond.Reason != "Paused" {
			t.Fatalf("acme integration condition = %#v, want paused reason", cond)
		}
	})

	t.Run("paused gateway cluster gets paused gateway integration condition", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
			Enabled:  true,
			Hostname: "bao.example.test",
			GatewayRef: openbaov1alpha1.GatewayReference{
				Name:      "shared-gateway",
				Namespace: "gateway-system",
			},
		}
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

		if err := reconciler.updateStatusForPaused(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForPaused() error = %v", err)
		}
		if cluster.Status.ObservedGeneration != cluster.Generation {
			t.Fatalf("observedGeneration = %d, want %d", cluster.Status.ObservedGeneration, cluster.Generation)
		}
		cond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
		if cond == nil || cond.Reason != "Paused" {
			t.Fatalf("gateway integration condition = %#v, want paused reason", cond)
		}
	})
}
