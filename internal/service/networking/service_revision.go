package networking

import (
	"strings"

	appsv1 "k8s.io/api/apps/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portworkload "github.com/dc-tec/openbao-operator/internal/port/workload"
)

func applyActiveServiceSelector(cluster *openbaov1alpha1.OpenBaoCluster, selector map[string]string) {
	if !isBlueGreenStrategy(cluster) {
		return
	}

	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
		greenRevision := strings.TrimSpace(cluster.Status.BlueGreen.GreenRevision)
		if greenRevision != "" {
			selector[constants.LabelOpenBaoRevision] = greenRevision
			return
		}
	}

	if stableRevision := stableServiceRevision(cluster); stableRevision != "" {
		selector[constants.LabelOpenBaoRevision] = stableRevision
		return
	}

	if cluster.Status.BlueGreen != nil {
		if controllerRevision := strings.TrimSpace(cluster.Status.BlueGreen.BlueControllerRevision); controllerRevision != "" {
			selector[appsv1.ControllerRevisionHashLabelKey] = controllerRevision
		}
	}
}

func stableServiceRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !isBlueGreenStrategy(cluster) {
		return ""
	}

	if cluster.Status.BlueGreen != nil {
		return strings.TrimSpace(cluster.Status.BlueGreen.BlueRevision)
	}

	return revision.OpenBaoClusterRevision(
		cluster.Spec.Version,
		resolvedSpecImage(cluster),
		cluster.Spec.Replicas,
	)
}

func isBlueGreenStrategy(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return portworkload.EffectiveStrategy(cluster) == openbaov1alpha1.UpdateStrategyBlueGreen
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
