//go:build integration
// +build integration

package integration

import (
	"strings"
	"testing"
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVAP_LockMaterializedOpenBaoClusterClaimSpec_DeniesSpecMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("create OpenBaoClusterClaim: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest openbaov1alpha1.OpenBaoClusterClaim
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(claim), &latest); err != nil {
			t.Fatalf("get OpenBaoClusterClaim: %v", err)
		}

		latest.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
			Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			LocalRef: &openbaov1alpha1.NamespacedReference{
				Namespace: namespace,
				Name:      claim.Name,
			},
		}
		if err := k8sClient.Status().Update(ctx, &latest); err != nil {
			t.Fatalf("update claim status to materialized: %v", err)
		}

		original := latest.DeepCopy()
		latest.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v2"}
		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "immutable after materialization") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny materialized OpenBaoClusterClaim spec mutation after retries")
}

func TestVAP_LockMaterializedOpenBaoClusterClaimSpec_AllowsControllerUpgradeMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	controllerClient := newControllerClient(t)
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("create OpenBaoClusterClaim: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest openbaov1alpha1.OpenBaoClusterClaim
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(claim), &latest); err != nil {
			t.Fatalf("get OpenBaoClusterClaim: %v", err)
		}

		latest.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
			Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			LocalRef: &openbaov1alpha1.NamespacedReference{
				Namespace: namespace,
				Name:      claim.Name,
			},
		}
		if err := k8sClient.Status().Update(ctx, &latest); err != nil {
			t.Fatalf("update claim status to materialized: %v", err)
		}

		var controllerView openbaov1alpha1.OpenBaoClusterClaim
		if err := controllerClient.Get(ctx, client.ObjectKeyFromObject(claim), &controllerView); err != nil {
			t.Fatalf("controller get OpenBaoClusterClaim: %v", err)
		}

		original := controllerView.DeepCopy()
		controllerView.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v2"}
		if err := controllerClient.Patch(ctx, &controllerView, client.MergeFrom(original)); err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var updated openbaov1alpha1.OpenBaoClusterClaim
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(claim), &updated); err != nil {
			t.Fatalf("get updated OpenBaoClusterClaim: %v", err)
		}
		if updated.Spec.ServiceProfileRef.Name != "standard-ha-v2" {
			t.Fatalf("serviceProfileRef = %q, want standard-ha-v2", updated.Spec.ServiceProfileRef.Name)
		}
		return
	}

	t.Fatalf("expected controller principal to be allowed to promote materialized OpenBaoClusterClaim service profile")
}

func TestVAP_LockMaterializedOpenBaoClusterClaimSpec_DeniesControllerPlacementMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	controllerClient := newControllerClient(t)
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
			ServiceParameters: &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
				Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
					Location: "tenant-payments-a",
				},
			},
		},
	}
	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("create OpenBaoClusterClaim: %v", err)
	}

	for attempt := 0; attempt < 25; attempt++ {
		var latest openbaov1alpha1.OpenBaoClusterClaim
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(claim), &latest); err != nil {
			t.Fatalf("get OpenBaoClusterClaim: %v", err)
		}

		latest.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
			Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			LocalRef: &openbaov1alpha1.NamespacedReference{
				Namespace: namespace,
				Name:      claim.Name,
			},
		}
		if err := k8sClient.Status().Update(ctx, &latest); err != nil {
			t.Fatalf("update claim status to materialized: %v", err)
		}

		var controllerView openbaov1alpha1.OpenBaoClusterClaim
		if err := controllerClient.Get(ctx, client.ObjectKeyFromObject(claim), &controllerView); err != nil {
			t.Fatalf("controller get OpenBaoClusterClaim: %v", err)
		}

		original := controllerView.DeepCopy()
		controllerView.Spec.ServiceParameters.Backup.Location = "tenant-payments-b"
		err := controllerClient.Patch(ctx, &controllerView, client.MergeFrom(original))
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "immutable after materialization") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny controller mutation of materialized OpenBaoClusterClaim backup location after retries")
}
