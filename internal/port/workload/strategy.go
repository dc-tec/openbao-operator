package workload

import (
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// DesiredStrategy returns the strategy requested by the cluster spec.
func DesiredStrategy(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.UpdateStrategyType {
	if cluster == nil || cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy == "" {
		return openbaov1alpha1.UpdateStrategyRollingUpdate
	}
	return cluster.Spec.Upgrade.Strategy
}

// EffectiveStrategy returns the last strategy accepted by the operator.
func EffectiveStrategy(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.UpdateStrategyType {
	if cluster != nil && cluster.Status.AcceptedUpgradeStrategy != "" {
		return cluster.Status.AcceptedUpgradeStrategy
	}
	return DesiredStrategy(cluster)
}

// StableVoterRevision returns the durable revision of the active voter StatefulSet.
func StableVoterRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return ""
	}
	return strings.TrimSpace(cluster.Status.BlueGreen.BlueRevision)
}

// StableVoterStatefulSetName returns the active voter StatefulSet name.
func StableVoterStatefulSetName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}
	revision := StableVoterRevision(cluster)
	if revision == "" {
		return cluster.Name
	}
	return fmt.Sprintf("%s-%s", cluster.Name, revision)
}

// StableVoterPodName returns a pod name in the active voter StatefulSet.
func StableVoterPodName(cluster *openbaov1alpha1.OpenBaoCluster, ordinal int32) string {
	return fmt.Sprintf("%s-%d", StableVoterStatefulSetName(cluster), ordinal)
}
