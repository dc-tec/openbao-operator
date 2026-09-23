package upgrade

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

// BlueGreenTargetReplicas returns the population selected for the active upgrade.
func BlueGreenTargetReplicas(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if status := cluster.Status.BlueGreen; status != nil && status.Phase != openbaov1alpha1.PhaseIdle && status.GreenReplicas > 0 {
		return status.GreenReplicas
	}
	return cluster.Spec.Replicas
}

// BlueGreenSourceReplicas returns the population of the stable Blue workload.
func BlueGreenSourceReplicas(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	if status := cluster.Status.BlueGreen; status != nil && status.BlueReplicas > 0 {
		return status.BlueReplicas
	}
	return cluster.Spec.Replicas
}
