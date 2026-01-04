//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/infra"
)

func TestInfraFullReconcile_StatefulSet_SSAAndIdempotency(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := createMinimalCluster(t, namespace, "infra-full-reconcile")
	createTLSSecret(t, namespace, cluster.Name)
	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
	})

	manager := infra.NewManager(k8sClient, k8sScheme, "openbao-operator-system", "", nil)

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, "", "", ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	stsKey := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	sts := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, stsKey, sts); err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != cluster.Spec.Replicas {
		t.Fatalf("expected StatefulSet replicas %d, got %#v", cluster.Spec.Replicas, sts.Spec.Replicas)
	}

	controllerRef := metav1.GetControllerOf(sts)
	if controllerRef == nil || controllerRef.UID != cluster.UID {
		t.Fatalf("expected StatefulSet to have controller owner reference to OpenBaoCluster %s/%s", cluster.Namespace, cluster.Name)
	}

	if !hasManagedFieldManager(sts, "openbao-operator") {
		t.Fatalf("expected StatefulSet to be SSA-applied by fieldManager openbao-operator")
	}

	// Simulate drift/other-actor writes:
	// - Add an external annotation (should be preserved by SSA).
	// - Set a RollingUpdate partition (UpgradeManager-managed; infra must not reset it).
	// - Change the main container image (infra should reconcile it back).
	drifted := sts.DeepCopy()
	original := sts.DeepCopy()

	if drifted.Annotations == nil {
		drifted.Annotations = map[string]string{}
	}
	drifted.Annotations["example.com/external-annotation"] = "true"
	drifted.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	drifted.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: ptr.To(int32(1)),
	}

	foundMain := false
	for i := range drifted.Spec.Template.Spec.Containers {
		if drifted.Spec.Template.Spec.Containers[i].Name == constants.ContainerBao {
			drifted.Spec.Template.Spec.Containers[i].Image = "openbao/openbao:broken"
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Fatalf("expected StatefulSet to have %q container", constants.ContainerBao)
	}

	if err := k8sClient.Patch(ctx, drifted, client.MergeFrom(original)); err != nil {
		t.Fatalf("patch drifted StatefulSet: %v", err)
	}

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, "", "", ""); err != nil {
		t.Fatalf("Reconcile() after drift error = %v", err)
	}

	updated := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, stsKey, updated); err != nil {
		t.Fatalf("get StatefulSet after reconcile: %v", err)
	}

	if updated.Annotations["example.com/external-annotation"] != "true" {
		t.Fatalf("expected external annotation to be preserved, got %#v", updated.Annotations)
	}

	if updated.Spec.UpdateStrategy.RollingUpdate == nil || updated.Spec.UpdateStrategy.RollingUpdate.Partition == nil || *updated.Spec.UpdateStrategy.RollingUpdate.Partition != 1 {
		t.Fatalf("expected RollingUpdate partition to be preserved, got %#v", updated.Spec.UpdateStrategy.RollingUpdate)
	}

	foundMain = false
	for i := range updated.Spec.Template.Spec.Containers {
		if updated.Spec.Template.Spec.Containers[i].Name == constants.ContainerBao {
			if updated.Spec.Template.Spec.Containers[i].Image != cluster.Spec.Image {
				t.Fatalf("expected main container image %q, got %q", cluster.Spec.Image, updated.Spec.Template.Spec.Containers[i].Image)
			}
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Fatalf("expected StatefulSet to have %q container", constants.ContainerBao)
	}

	if !hasManagedFieldManager(updated, "openbao-operator") {
		t.Fatalf("expected StatefulSet to remain SSA-managed by fieldManager openbao-operator")
	}

	// One more pass should be safe/idempotent.
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, "", "", ""); err != nil {
		t.Fatalf("Reconcile() third call error = %v", err)
	}
}

func hasManagedFieldManager(obj metav1.Object, manager string) bool {
	for _, entry := range obj.GetManagedFields() {
		if entry.Manager == manager {
			return true
		}
	}
	return false
}
