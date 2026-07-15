package openbaocluster

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	upgradesvc "github.com/dc-tec/openbao-operator/internal/service/upgrade"
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

	phase := openbaov1alpha1.PhaseIdle
	if cluster.Status.BlueGreen != nil {
		phase = cluster.Status.BlueGreen.Phase
	}

	if blueGreenPhaseRequiresReadReplicaStageDown(phase) {
		return true
	}

	if cluster.Status.OperationLock == nil {
		return false
	}

	switch cluster.Status.OperationLock.Operation {
	case openbaov1alpha1.ClusterOperationRestore:
		return true
	case openbaov1alpha1.ClusterOperationUpgrade:
		return upgradesvc.EffectiveStrategy(cluster) == openbaov1alpha1.UpdateStrategyBlueGreen &&
			(phase == "" || phase == openbaov1alpha1.PhaseIdle)
	default:
		return false
	}
}

func blueGreenPhaseRequiresReadReplicaStageDown(phase openbaov1alpha1.BlueGreenPhase) bool {
	switch phase {
	case openbaov1alpha1.PhaseDeployingGreen,
		openbaov1alpha1.PhaseJoiningMesh,
		openbaov1alpha1.PhaseSyncing,
		openbaov1alpha1.PhasePromoting,
		openbaov1alpha1.PhaseDemotingBlue,
		openbaov1alpha1.PhaseCleanup,
		openbaov1alpha1.PhaseRollingBack,
		openbaov1alpha1.PhaseRollbackCleanup:
		return true
	default:
		return false
	}
}
