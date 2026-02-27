package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/statusops"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// StatusDependencies provides dependencies needed for status state observation.
type StatusDependencies struct {
	Reader client.Reader
}

// StatusState is the app-layer status observation model used by controller status helpers.
type StatusState = statusops.StatusState

// GatherStatusState reads current cluster state needed for status reconciliation.
func GatherStatusState(
	ctx context.Context,
	logger logr.Logger,
	deps StatusDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*StatusState, error) {
	if deps.Reader == nil {
		return nil, fmt.Errorf("status reader dependency is required")
	}
	return statusops.GatherState(ctx, logger, deps.Reader, cluster, statusops.LabelConfig{
		AppInstanceKey:       constants.LabelAppInstance,
		AppManagedByKey:      constants.LabelAppManagedBy,
		AppManagedByValue:    constants.LabelValueAppManagedByOpenBaoOperator,
		OpenBaoClusterKey:    constants.LabelOpenBaoCluster,
		OpenBaoComponentKey:  constants.LabelOpenBaoComponent,
		BackupComponentValue: constants.ComponentBackup,
		AppNameKey:           constants.LabelAppName,
		AppNameValue:         constants.LabelValueAppNameOpenBao,
		OpenBaoRevisionKey:   constants.LabelOpenBaoRevision,
	})
}

// ObservedVersionFromPods derives the observed workload version from pod labels.
func ObservedVersionFromPods(state *StatusState) string {
	return statusops.ObservedVersionFromPods(state)
}

// ReconcileCurrentVersion aligns status.currentVersion with observed workload version.
func ReconcileCurrentVersion(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, state *StatusState, observedVersion string) {
	statusops.ReconcileCurrentVersion(logger, cluster, state, observedVersion)
}

// MaybeAdvanceCurrentVersionForBlueGreen advances currentVersion on completed blue/green upgrades.
func MaybeAdvanceCurrentVersionForBlueGreen(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, observedVersion string) {
	statusops.MaybeAdvanceCurrentVersionForBlueGreen(logger, cluster, observedVersion)
}

// ShouldWarnSelfInitDisabled returns whether reconciliation should emit the root-token warning.
func ShouldWarnSelfInitDisabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return statusops.ShouldWarnSelfInitDisabled(cluster)
}
