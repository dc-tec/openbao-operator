package bluegreen

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func shouldRestoreSteadyReadReplicas(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Replicas > 0
}

func beginSteadyReadReplicaRestore(
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) {
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return
	}

	if greenRevision := cluster.Status.BlueGreen.GreenRevision; greenRevision != "" {
		cluster.Status.BlueGreen.BlueRevision = greenRevision
		cluster.Status.BlueGreen.BlueControllerRevision = ""
	}
	if cluster.Spec.Image != "" {
		cluster.Status.BlueGreen.BlueImage = cluster.Spec.Image
	}
	cluster.Status.BlueGreen.GreenRevision = ""
	cluster.Status.BlueGreen.ManualPromotionRequired = false
	maybeResetBlueGreenRollbackState(cluster)

	logger.Info("Blue/green cleanup complete; restoring steady read replicas before finalizing upgrade")
}

func maybeResetBlueGreenRollbackState(cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster == nil || cluster.Status.BlueGreen == nil || cluster.Status.BlueGreen.RollbackStartTime == nil {
		return
	}

	cluster.Status.BlueGreen.RollbackReason = ""
	cluster.Status.BlueGreen.RollbackStartTime = nil
	cluster.Status.BlueGreen.RollbackAttempt = 0
}

func readReplicaRestoreComplete(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if !shouldRestoreSteadyReadReplicas(cluster) {
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

	return conditionTrue(cluster, openbaov1alpha1.ConditionReadReplicasReady) &&
		conditionTrue(cluster, openbaov1alpha1.ConditionReadServingAvailable) &&
		conditionTrue(cluster, openbaov1alpha1.ConditionRaftMembershipReady)
}

func conditionTrue(cluster *openbaov1alpha1.OpenBaoCluster, condType openbaov1alpha1.ConditionType) bool {
	if cluster == nil {
		return false
	}
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(condType))
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func (m *Manager) handlePhaseRestoringReadReplicas(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (phaseOutcome, error) {
	if cluster.Status.BlueGreen == nil {
		return phaseOutcome{}, nil
	}

	if !shouldRestoreSteadyReadReplicas(cluster) {
		return m.finalizeCompletedBlueGreenUpgrade(ctx, logger, cluster, false)
	}

	if !readReplicaRestoreComplete(cluster) {
		readyReplicas := int32(0)
		registeredReplicas := int32(0)
		if cluster.Status.ReadReplicas != nil {
			readyReplicas = cluster.Status.ReadReplicas.ReadyReplicas
			registeredReplicas = cluster.Status.ReadReplicas.RegisteredReplicas
		}

		logger.Info(
			"Waiting for steady read replicas to restore before finalizing blue/green upgrade",
			"desiredReadReplicas", cluster.Spec.ReadReplicas.Replicas,
			"readyReadReplicas", readyReplicas,
			"registeredReadReplicas", registeredReplicas,
			"readReplicasReady", conditionTrue(cluster, openbaov1alpha1.ConditionReadReplicasReady),
			"readServingAvailable", conditionTrue(cluster, openbaov1alpha1.ConditionReadServingAvailable),
			"raftMembershipReady", conditionTrue(cluster, openbaov1alpha1.ConditionRaftMembershipReady),
		)
		return requeueAfterOutcome(constants.RequeueShort), nil
	}

	return m.finalizeCompletedBlueGreenUpgrade(ctx, logger, cluster, false)
}
