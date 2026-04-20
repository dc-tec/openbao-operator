package openbaocluster

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestPatchStatusSSAPersistsObservedGeneration(t *testing.T) {
	scheme := newOpenBaoClusterTestScheme(t)
	cluster := newOpenBaoClusterStatusTestObject()
	cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	cluster.Status.ReadyReplicas = cluster.Spec.Replicas
	cluster.Status.ReadReplicas = &openbaov1alpha1.ReadReplicaStatus{
		DesiredReplicas: 2,
		ReadyReplicas:   1,
		HealthyReplicas: 1,
		Storage: openbaov1alpha1.ReadReplicaStorageStatus{
			DesiredPVCs:      2,
			BoundPVCs:        1,
			StorageClassName: "fast-ssd",
		},
	}
	cluster.Status.CurrentVersion = cluster.Spec.Version

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(cluster).
		Build()

	reconciler := &OpenBaoClusterReconciler{Client: k8sClient}
	if err := reconciler.patchStatusSSA(context.Background(), cluster); err != nil {
		t.Fatalf("patchStatusSSA() error = %v", err)
	}

	if cluster.Status.ObservedGeneration != cluster.Generation {
		t.Fatalf("cluster.Status.ObservedGeneration = %d, want %d", cluster.Status.ObservedGeneration, cluster.Generation)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if updated.Status.ObservedGeneration != cluster.Generation {
		t.Fatalf("persisted observedGeneration = %d, want %d", updated.Status.ObservedGeneration, cluster.Generation)
	}
	if updated.Status.Phase != openbaov1alpha1.ClusterPhaseRunning {
		t.Fatalf("persisted phase = %s, want %s", updated.Status.Phase, openbaov1alpha1.ClusterPhaseRunning)
	}
	if updated.Status.ReadReplicas == nil {
		t.Fatalf("persisted readReplicas = nil, want non-nil")
	}
	if updated.Status.ReadReplicas.Storage.StorageClassName != "fast-ssd" {
		t.Fatalf("persisted readReplicas.storage.storageClassName = %q, want %q", updated.Status.ReadReplicas.Storage.StorageClassName, "fast-ssd")
	}
	if updated.Status.ReadReplicas.HealthyReplicas != 1 {
		t.Fatalf("persisted readReplicas.healthyReplicas = %d, want 1", updated.Status.ReadReplicas.HealthyReplicas)
	}
}
