package openbaocluster

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (r *OpenBaoClusterReconciler) determineStatusRequeue(logger logr.Logger, state *clusterState, original, cluster *openbaov1alpha1.OpenBaoCluster) ctrl.Result {
	if state == nil || original == nil || cluster == nil {
		return ctrl.Result{}
	}

	previousReadyReplicas := original.Status.ReadyReplicas
	readyReplicasChanged := state.ReadyReplicas != previousReadyReplicas
	desiredReadReplicas := desiredReadReplicaCount(cluster)
	previousReadReadyReplicas := int32(0)
	previousReadRegisteredReplicas := int32(0)
	if original.Status.ReadReplicas != nil {
		previousReadReadyReplicas = original.Status.ReadReplicas.ReadyReplicas
		previousReadRegisteredReplicas = original.Status.ReadReplicas.RegisteredReplicas
	}
	readReadyReplicasChanged := state.ReadReplicaReadyReplicas != previousReadReadyReplicas
	readRegisteredReplicasChanged := state.ReadReplicaRegisteredReplicas != previousReadRegisteredReplicas

	if state.StatusStale {
		logger.V(1).Info("StatefulSet status may be stale; requeuing to check status")
		return ctrl.Result{RequeueAfter: constants.RequeueShort}
	}

	if !state.Available && state.ReadyReplicas > 0 {
		logger.V(1).Info("Not all replicas are ready; requeuing to check status",
			"readyReplicas", state.ReadyReplicas,
			"desiredReplicas", cluster.Spec.Replicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}
	}

	if state.Available && readyReplicasChanged {
		logger.V(1).Info("All replicas became ready; requeuing once to ensure status is persisted",
			"readyReplicas", state.ReadyReplicas,
			"previousReadyReplicas", previousReadyReplicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}
	}

	if desiredReadReplicas > 0 && state.ReadReplicaReadyReplicas != desiredReadReplicas {
		logger.V(1).Info("Read replica pool is still converging; requeuing to refresh status",
			"readReplicaReadyReplicas", state.ReadReplicaReadyReplicas,
			"desiredReadReplicas", desiredReadReplicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}
	}

	if desiredReadReplicas > 0 && state.ReadReplicaReadyReplicas == desiredReadReplicas {
		if !state.ReadServingKnown || !state.ReadServingAvailable {
			logger.V(1).Info("Read replica serving state has not fully converged; requeuing to refresh status",
				"readServingKnown", state.ReadServingKnown,
				"readServingAvailable", state.ReadServingAvailable,
				"desiredReadReplicas", desiredReadReplicas)
			return ctrl.Result{RequeueAfter: constants.RequeueShort}
		}
		if !state.ReadReplicaMembershipKnown || state.ReadReplicaRegisteredReplicas != desiredReadReplicas {
			logger.V(1).Info("Read replica raft membership has not fully converged; requeuing to refresh status",
				"readReplicaMembershipKnown", state.ReadReplicaMembershipKnown,
				"readReplicaRegisteredReplicas", state.ReadReplicaRegisteredReplicas,
				"desiredReadReplicas", desiredReadReplicas)
			return ctrl.Result{RequeueAfter: constants.RequeueShort}
		}
	}

	if desiredReadReplicas > 0 && (readReadyReplicasChanged || readRegisteredReplicasChanged) {
		logger.V(1).Info("Read replica status changed; requeuing once to ensure status is persisted",
			"readReplicaReadyReplicas", state.ReadReplicaReadyReplicas,
			"previousReadReplicaReadyReplicas", previousReadReadyReplicas,
			"readReplicaRegisteredReplicas", state.ReadReplicaRegisteredReplicas,
			"previousReadReplicaRegisteredReplicas", previousReadRegisteredReplicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}
	}

	return ctrl.Result{}
}
