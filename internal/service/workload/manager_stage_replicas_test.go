package workload

import "testing"

func TestClusterForStatefulSetSpec_UsesStagedReplicas(t *testing.T) {
	cluster := newMinimalCluster("staged-scale-down", "default")
	cluster.Spec.Replicas = 1

	spec := StatefulSetSpec{
		Name:     cluster.Name,
		Replicas: 2,
	}

	got := clusterForStatefulSetSpec(cluster, spec)
	if got == cluster {
		t.Fatalf("expected staged cluster copy when replicas differ")
	}
	if got.Spec.Replicas != spec.Replicas {
		t.Fatalf("staged cluster replicas = %d, want %d", got.Spec.Replicas, spec.Replicas)
	}
	if cluster.Spec.Replicas != 1 {
		t.Fatalf("original cluster replicas mutated to %d, want 1", cluster.Spec.Replicas)
	}
}

func TestClusterForStatefulSetSpec_ReusesClusterWhenReplicasMatch(t *testing.T) {
	cluster := newMinimalCluster("steady-state", "default")
	cluster.Spec.Replicas = 2

	spec := StatefulSetSpec{
		Name:     cluster.Name,
		Replicas: 2,
	}

	got := clusterForStatefulSetSpec(cluster, spec)
	if got != cluster {
		t.Fatalf("expected original cluster when replicas already match")
	}
}
