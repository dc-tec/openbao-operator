package networking

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func activeServiceRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !isBlueGreenStrategy(cluster) {
		return ""
	}

	stableRevision := stableServiceRevision(cluster)
	if stableRevision == "" {
		return ""
	}
	if cluster.Status.BlueGreen == nil {
		return stableRevision
	}

	if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
		greenRevision := strings.TrimSpace(cluster.Status.BlueGreen.GreenRevision)
		if greenRevision != "" {
			return greenRevision
		}
	}

	return stableRevision
}

func stableServiceRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !isBlueGreenStrategy(cluster) {
		return ""
	}

	if cluster.Status.BlueGreen != nil {
		blueRevision := strings.TrimSpace(cluster.Status.BlueGreen.BlueRevision)
		if blueRevision != "" {
			return blueRevision
		}
	}

	return revision.OpenBaoClusterRevision(
		cluster.Spec.Version,
		resolvedSpecImage(cluster),
		cluster.Spec.Replicas,
	)
}

func isBlueGreenStrategy(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.Upgrade != nil &&
		cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen
}

func resolvedSpecImage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil {
		return ""
	}

	specImage := strings.TrimSpace(cluster.Spec.Image)
	if specImage != "" {
		return specImage
	}

	return constants.GetOpenBaoImage(cluster.Spec.Version)
}
