package infra

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// IsBlueGreenStrategy returns true when the cluster is configured for blue/green upgrades.
func IsBlueGreenStrategy(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.Upgrade != nil &&
		cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen
}

// BlueGreenStableRevision returns the currently-active stable revision ("Blue").
// If status does not yet contain BlueRevision, a deterministic spec-derived revision is returned.
func BlueGreenStableRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if !IsBlueGreenStrategy(cluster) {
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

// BlueGreenActiveRevision returns the traffic-active revision for selectors.
// Traffic remains on Blue by default and switches to Green only in Cleanup phase.
func BlueGreenActiveRevision(cluster *openbaov1alpha1.OpenBaoCluster) string {
	blueRevision := BlueGreenStableRevision(cluster)
	if blueRevision == "" {
		return ""
	}
	if cluster.Status.BlueGreen == nil {
		return blueRevision
	}

	if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
		greenRevision := strings.TrimSpace(cluster.Status.BlueGreen.GreenRevision)
		if greenRevision != "" {
			return greenRevision
		}
	}
	return blueRevision
}

// EnsureBlueGreenStatus bootstraps and repairs status.BlueGreen state used by reconcilers.
// It is safe to call repeatedly and only mutates in-memory cluster status.
func EnsureBlueGreenStatus(ctx context.Context, logger logr.Logger, c client.Reader, cluster *openbaov1alpha1.OpenBaoCluster) {
	if !IsBlueGreenStrategy(cluster) || c == nil || cluster == nil {
		return
	}

	inferredRevision, inferredImage, inferErr := InferActiveRevisionFromPods(ctx, c, cluster)
	if inferErr != nil {
		logger.Error(inferErr, "Failed to infer active revision from pods; falling back to status/spec")
	}

	inferredRevision = strings.TrimSpace(inferredRevision)
	inferredImage = strings.TrimSpace(inferredImage)

	if cluster.Status.BlueGreen == nil {
		blueRevision := inferredRevision
		if blueRevision == "" {
			blueRevision = revision.OpenBaoClusterRevision(
				cluster.Spec.Version,
				resolvedSpecImage(cluster),
				cluster.Spec.Replicas,
			)
		}

		cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
			Phase:        openbaov1alpha1.PhaseIdle,
			BlueRevision: blueRevision,
			BlueImage:    resolveBlueImage(cluster, inferredImage),
		}
		return
	}

	if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
		return
	}
	if cluster.Status.BlueGreen.BlueRevision != "" && cluster.Status.CurrentVersion == cluster.Spec.Version {
		return
	}

	if inferredRevision != "" && inferredRevision != cluster.Status.BlueGreen.BlueRevision {
		logger.Info("Correcting BlueRevision from active pods", "from", cluster.Status.BlueGreen.BlueRevision, "to", inferredRevision)
		cluster.Status.BlueGreen.BlueRevision = inferredRevision
	}

	if inferredImage != "" && (cluster.Status.BlueGreen.BlueImage == "" || cluster.Status.BlueGreen.BlueImage != inferredImage) {
		logger.Info("Correcting BlueImage from active pods", "from", cluster.Status.BlueGreen.BlueImage, "to", inferredImage)
		cluster.Status.BlueGreen.BlueImage = inferredImage
		return
	}

	if strings.TrimSpace(cluster.Status.BlueGreen.BlueImage) == "" {
		cluster.Status.BlueGreen.BlueImage = resolveBlueImage(cluster, "")
	}
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

func resolveBlueImage(cluster *openbaov1alpha1.OpenBaoCluster, inferredImage string) string {
	if image := strings.TrimSpace(inferredImage); image != "" {
		return image
	}

	// During upgrade, CurrentVersion tracks the active ("Blue") version.
	if cluster.Status.CurrentVersion != "" && cluster.Status.CurrentVersion != cluster.Spec.Version {
		return constants.GetOpenBaoImage(cluster.Status.CurrentVersion)
	}

	return resolvedSpecImage(cluster)
}
