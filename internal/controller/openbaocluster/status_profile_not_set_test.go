package openbaocluster

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestUpdateStatusForProfileNotSet(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)

	t.Run("missing profile marks cluster blocked", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = ""
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
		userAccess := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionUserAccessBootstrap))
		if userAccess == nil {
			t.Fatalf("expected condition %s", openbaov1alpha1.ConditionUserAccessBootstrap)
		}
		cloudUnseal := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionCloudUnsealIdentityReady))
		if cloudUnseal == nil || cloudUnseal.Reason != ReasonProfileNotSet {
			t.Fatalf("cloud unseal identity condition = %#v, want reason %q", cloudUnseal, ReasonProfileNotSet)
		}
		apiServer := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionAPIServerNetworkReady))
		if apiServer == nil || apiServer.Reason != ReasonProfileNotSet {
			t.Fatalf("api server network condition = %#v, want reason %q", apiServer, ReasonProfileNotSet)
		}
	})

	t.Run("missing profile marks acme integration as unknown", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = ""
		cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
		cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"}
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
			WithObjects(cluster).
			Build()
		reconciler := &OpenBaoClusterReconciler{Client: k8sClient}

		if err := reconciler.updateStatusForProfileNotSet(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForProfileNotSet() error = %v", err)
		}
		cond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
		if cond == nil || cond.Reason != ReasonProfileNotSet {
			t.Fatalf("acme integration condition = %#v, want reason %q", cond, ReasonProfileNotSet)
		}
	})

	t.Run("missing profile marks gateway integration as unknown", func(t *testing.T) {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = ""
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

		if err := reconciler.updateStatusForProfileNotSet(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("updateStatusForProfileNotSet() error = %v", err)
		}
		cond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionGatewayIntegrationReady))
		if cond == nil || cond.Reason != ReasonProfileNotSet {
			t.Fatalf("gateway integration condition = %#v, want reason %q", cond, ReasonProfileNotSet)
		}
	})
}
