package upgrade

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portworkload "github.com/dc-tec/openbao-operator/internal/port/workload"
)

// DesiredStrategy returns the strategy requested by the cluster spec.
func DesiredStrategy(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.UpdateStrategyType {
	return portworkload.DesiredStrategy(cluster)
}

// EffectiveStrategy returns the last strategy accepted by the operator. This
// keeps controllers on the previous strategy while a requested transition is
// still blocked by an active operation or an unmet safety prerequisite.
func EffectiveStrategy(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.UpdateStrategyType {
	return portworkload.EffectiveStrategy(cluster)
}

// StableVoterRevision returns the durable revision of the active voter
// StatefulSet. An empty revision identifies the original unrevisioned
// StatefulSet used by rolling clusters.
func StableVoterRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return portworkload.StableVoterRevision(cluster)
}

// StableVoterStatefulSetName returns the active voter StatefulSet name without
// changing it merely because a different future upgrade strategy was selected.
func StableVoterStatefulSetName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return portworkload.StableVoterStatefulSetName(cluster)
}

// StableVoterPodName returns a pod name in the active voter StatefulSet.
func StableVoterPodName(cluster *openbaov1alpha1.OpenBaoCluster, ordinal int32) string {
	return portworkload.StableVoterPodName(cluster, ordinal)
}
