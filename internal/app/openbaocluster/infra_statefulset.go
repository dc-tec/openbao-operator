package openbaocluster

import (
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
)

// computeStatefulSetSpec computes the StatefulSetSpec from the cluster and verified image digests.
func (r *infraReconciler) computeStatefulSetSpec(
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	verifiedImageDigest string,
	verifiedInitContainerDigest string,
) inframanager.StatefulSetSpec {
	spec := inframanager.StatefulSetSpec{
		Image:              verifiedImageDigest,
		InitContainerImage: verifiedInitContainerDigest,
		Replicas:           cluster.Spec.Replicas,
		DisableSelfInit:    false,
		SkipReconciliation: false,
	}

	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		spec.Revision = inframanager.BlueGreenStableRevision(cluster)
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
		spec.Revision = ""
	}

	if spec.Revision == "" {
		spec.Name = cluster.Name
	} else {
		spec.Name = fmt.Sprintf("%s-%s", cluster.Name, spec.Revision)
	}

	return spec
}
