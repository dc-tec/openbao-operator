package openbaocluster

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func (r *OpenBaoClusterReconciler) determineStatusRequeue(logger logr.Logger, state *clusterState, original, cluster *openbaov1alpha1.OpenBaoCluster) ctrl.Result {
	if state == nil || original == nil || cluster == nil {
		return ctrl.Result{}
	}

	previousReadyReplicas := original.Status.ReadyReplicas
	readyReplicasChanged := state.ReadyReplicas != previousReadyReplicas

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

	return ctrl.Result{}
}
