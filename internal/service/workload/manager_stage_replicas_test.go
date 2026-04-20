package workload

import (
	"testing"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestDesiredStatefulSetReplicas_UsesStagedReplicasForVoters(t *testing.T) {
	cluster := newMinimalCluster("staged-scale-down", "default")
	cluster.Spec.Replicas = 1

	spec := StatefulSetSpec{
		Name:     cluster.Name,
		Replicas: 2,
	}

	got := desiredStatefulSetReplicas(cluster, true, spec)
	if got != spec.Replicas {
		t.Fatalf("desiredStatefulSetReplicas() = %d, want %d", got, spec.Replicas)
	}
}

func TestDesiredStatefulSetReplicas_ReadReplicasStayZeroBeforeInitialization(t *testing.T) {
	cluster := newMinimalCluster("steady-state", "default")
	cluster.Spec.Replicas = 2

	spec := StatefulSetSpec{
		Name:     cluster.Name + "-read",
		Pool:     constants.LabelValueOpenBaoWorkloadPoolReadReplica,
		Replicas: 2,
	}

	got := desiredStatefulSetReplicas(cluster, false, spec)
	if got != 0 {
		t.Fatalf("desiredStatefulSetReplicas() = %d, want 0", got)
	}
}
