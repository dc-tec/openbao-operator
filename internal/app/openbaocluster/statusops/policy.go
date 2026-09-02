package statusops

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// PolicyInput contains the observed and persisted state used to compute the
// status fields owned by the status controller.
type PolicyInput struct {
	Original       *openbaov1alpha1.OpenBaoCluster
	Cluster        *openbaov1alpha1.OpenBaoCluster
	State          *StatusState
	AdmissionState *admission.Status
	Now            metav1.Time
}

// ApplyPolicy computes normal status fields and a requeue decision from observed state.
func ApplyPolicy(logger logr.Logger, input PolicyInput) recon.Result {
	applyAllConditions(input.Cluster, input.State, input.AdmissionState, input.Now)

	input.Cluster.Status.ReadyReplicas = input.State.ReadyReplicas
	input.Cluster.Status.ReadReplicas = buildReadReplicaStatus(input.Cluster, input.State)
	input.Cluster.Status.ActiveLeader = input.State.LeaderName
	input.Cluster.Status.Phase = computePhase(input.State)

	observedVersion := ObservedVersionFromPods(input.State)
	ReconcileCurrentVersion(logger, input.Cluster, input.State, observedVersion)
	MaybeAdvanceCurrentVersionForBlueGreen(logger, input.Cluster, observedVersion)

	return determineStatusRequeue(logger, input.State, input.Original, input.Cluster)
}

func buildReadReplicaStatus(
	cluster *openbaov1alpha1.OpenBaoCluster,
	state *StatusState,
) *openbaov1alpha1.ReadReplicaStatus {
	if cluster.Spec.ReadReplicas == nil {
		return nil
	}

	status := &openbaov1alpha1.ReadReplicaStatus{
		DesiredReplicas: cluster.Spec.ReadReplicas.Replicas,
	}

	if state != nil {
		status.ReadyReplicas = state.ReadReplicaReadyReplicas
		status.RegisteredReplicas = state.ReadReplicaRegisteredReplicas
		status.HealthyReplicas = state.ReadReplicaHealthyReplicas
		status.Storage.DesiredPVCs = cluster.Spec.ReadReplicas.Replicas
		status.Storage.BoundPVCs = int32(state.ReadReplicaDataPVCCount)
		switch {
		case len(state.ReadReplicaDataPVCStorageClassNames) == 1:
			status.Storage.StorageClassName = state.ReadReplicaDataPVCStorageClassNames[0]
		case cluster.Spec.ReadReplicas.Storage != nil && cluster.Spec.ReadReplicas.Storage.StorageClassName != nil:
			status.Storage.StorageClassName = *cluster.Spec.ReadReplicas.Storage.StorageClassName
		}
		return status
	}

	status.Storage.DesiredPVCs = cluster.Spec.ReadReplicas.Replicas
	if cluster.Spec.ReadReplicas.Storage != nil && cluster.Spec.ReadReplicas.Storage.StorageClassName != nil {
		status.Storage.StorageClassName = *cluster.Spec.ReadReplicas.Storage.StorageClassName
	}
	return status
}
