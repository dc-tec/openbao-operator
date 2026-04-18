package openbaocluster

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func applyOperationalReadReplicaStageDown(cluster *openbaov1alpha1.OpenBaoCluster, readSpec *workloadsvc.StatefulSetSpec) bool {
	if cluster == nil || readSpec == nil {
		return false
	}
	if !shouldStageSteadyReadReplicasDown(cluster) {
		return false
	}
	if readSpec.Replicas == 0 {
		return false
	}

	readSpec.Replicas = 0
	return true
}

func shouldStageSteadyReadReplicasDown(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.ReadReplicas == nil || cluster.Spec.ReadReplicas.Replicas == 0 {
		return false
	}

	if cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.Phase != "" &&
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		return true
	}

	if cluster.Status.OperationLock == nil {
		return false
	}

	switch cluster.Status.OperationLock.Operation {
	case openbaov1alpha1.ClusterOperationRestore:
		return true
	case openbaov1alpha1.ClusterOperationUpgrade:
		return cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen
	default:
		return false
	}
}
