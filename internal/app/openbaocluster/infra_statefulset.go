package openbaocluster

import (
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

// computeStatefulSetSpec computes the StatefulSetSpec from the cluster and verified image digests.
func (r *infraReconciler) computeStatefulSetSpec(
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	verifiedImageDigest string,
	verifiedInitContainerDigest string,
) workloadsvc.StatefulSetSpec {
	spec := workloadsvc.StatefulSetSpec{
		Pool:               constants.LabelValueOpenBaoWorkloadPoolVoter,
		Image:              verifiedImageDigest,
		InitContainerImage: verifiedInitContainerDigest,
		Replicas:           cluster.Spec.Replicas,
		RestoreRevision:    clusterRestoreRevision(cluster),
		DisableSelfInit:    false,
		SkipReconciliation: false,
	}

	if workloadsvc.IsBlueGreenStrategy(cluster) {
		spec.Revision = workloadsvc.BlueGreenStableRevision(cluster)
		if spec.Revision == "" {
			spec.Name = cluster.Name
		} else {
			spec.Name = fmt.Sprintf("%s-%s", cluster.Name, spec.Revision)
		}
		if cluster.Status.BlueGreen != nil &&
			(cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup) {
			logger.Info("Skipping Blue StatefulSet reconciliation during cleanup phase",
				"phase", cluster.Status.BlueGreen.Phase,
				"blueRevision", cluster.Status.BlueGreen.BlueRevision)
			spec.SkipReconciliation = true
			return spec
		}
	} else {
		spec.Revision = upgrade.StableVoterRevision(cluster)
		spec.Name = upgrade.StableVoterStatefulSetName(cluster)
	}

	return spec
}

func (r *infraReconciler) computeReadReplicaStatefulSetSpec(
	cluster *openbaov1alpha1.OpenBaoCluster,
	verifiedImageDigest string,
	verifiedInitContainerDigest string,
) workloadsvc.StatefulSetSpec {
	replicas := int32(0)
	if cluster.Spec.ReadReplicas != nil {
		replicas = cluster.Spec.ReadReplicas.Replicas
	}

	return workloadsvc.StatefulSetSpec{
		Name:               resourceidentity.ReadReplicaStatefulSetName(cluster),
		Pool:               constants.LabelValueOpenBaoWorkloadPoolReadReplica,
		Image:              verifiedImageDigest,
		InitContainerImage: verifiedInitContainerDigest,
		Replicas:           replicas,
		RestoreRevision:    clusterRestoreRevision(cluster),
		DisableSelfInit:    true,
		SkipReconciliation: cluster.Spec.ReadReplicas == nil,
	}
}

func clusterRestoreRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Status.Restore == nil {
		return ""
	}
	return cluster.Status.Restore.UID
}
