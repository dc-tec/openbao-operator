package restore

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func shouldWaitForSteadyReadReplicaRestore(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Replicas > 0
}

func steadyReadReplicaRestoreComplete(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if !shouldWaitForSteadyReadReplicaRestore(cluster) {
		return true
	}
	if cluster == nil || cluster.Status.ReadReplicas == nil {
		return false
	}

	desired := cluster.Spec.ReadReplicas.Replicas
	if cluster.Status.ReadReplicas.ReadyReplicas != desired ||
		cluster.Status.ReadReplicas.RegisteredReplicas != desired {
		return false
	}

	return restoreConditionTrue(cluster, openbaov1alpha1.ConditionReadReplicasReady) &&
		restoreConditionTrue(cluster, openbaov1alpha1.ConditionReadServingAvailable) &&
		restoreConditionTrue(cluster, openbaov1alpha1.ConditionRaftMembershipReady)
}

func restoreConditionTrue(cluster *openbaov1alpha1.OpenBaoCluster, condType openbaov1alpha1.ConditionType) bool {
	if cluster == nil {
		return false
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(condType))
	return condition != nil && condition.Status == metav1.ConditionTrue
}
