package statusops

import (
	"strings"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/revision"
	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

// ReconcileCurrentVersion aligns CurrentVersion with observed workload version
// while preserving upgrade safety guards.
func ReconcileCurrentVersion(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, state *StatusState, observedVersion string) {
	if !cluster.Status.Initialized {
		return
	}

	if cluster.Status.CurrentVersion == "" && observedVersion != "" {
		cluster.Status.CurrentVersion = observedVersion
		logger.Info("Set initial CurrentVersion from running pod labels", "version", observedVersion)
		return
	}

	if state == nil {
		return
	}

	if state.RollingUpgradeInProgress || state.BlueGreenInProgress {
		return
	}

	if observedVersion == "" || cluster.Status.CurrentVersion == "" || cluster.Status.CurrentVersion == observedVersion {
		return
	}

	change, err := upgrade.CompareVersions(cluster.Status.CurrentVersion, observedVersion)
	if err != nil {
		logger.V(1).Info("Skipping CurrentVersion correction due to unparsable version",
			"currentVersion", cluster.Status.CurrentVersion,
			"observedVersion", observedVersion,
			"error", err)
		return
	}
	if change == upgrade.VersionChangeDowngrade {
		logger.V(1).Info("Ignoring CurrentVersion regression from pod labels",
			"currentVersion", cluster.Status.CurrentVersion,
			"observedVersion", observedVersion)
		return
	}

	from := cluster.Status.CurrentVersion
	cluster.Status.CurrentVersion = observedVersion
	logger.Info("Corrected CurrentVersion from running pod labels",
		"fromVersion", from,
		"toVersion", observedVersion)
}

// MaybeAdvanceCurrentVersionForBlueGreen updates CurrentVersion when blue/green
// status indicates completion and observed version checks pass.
func MaybeAdvanceCurrentVersionForBlueGreen(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, observedVersion string) {
	if cluster.Status.BlueGreen == nil ||
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle ||
		cluster.Status.CurrentVersion == "" ||
		cluster.Status.CurrentVersion == cluster.Spec.Version ||
		cluster.Spec.Upgrade == nil ||
		cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen {
		return
	}

	currentSpecRevision := revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
	if cluster.Status.BlueGreen.BlueRevision != currentSpecRevision {
		return
	}

	if observedVersion != "" && strings.TrimSpace(observedVersion) != strings.TrimSpace(cluster.Spec.Version) {
		logger.V(1).Info("BlueGreen revision matches but running pods do not report target version; skipping CurrentVersion update",
			"observedVersion", observedVersion,
			"targetVersion", cluster.Spec.Version,
			"revision", currentSpecRevision)
		return
	}

	from := cluster.Status.CurrentVersion
	cluster.Status.CurrentVersion = cluster.Spec.Version
	logger.Info("Detected BlueGreen upgrade completion, updated CurrentVersion",
		"fromVersion", from,
		"toVersion", cluster.Spec.Version,
		"revision", currentSpecRevision)
}

// ShouldWarnSelfInitDisabled reports whether status reconciliation should emit
// the root-token warning.
func ShouldWarnSelfInitDisabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled
}

// ObservedVersionFromPods derives an observed OpenBao version from pod labels.
func ObservedVersionFromPods(state *StatusState) string {
	if state == nil || len(state.Pods) == 0 {
		return ""
	}

	// Prefer leader version when leadership is unambiguous.
	if state.LeaderCount == 1 && strings.TrimSpace(state.LeaderName) != "" {
		for i := range state.Pods {
			pod := &state.Pods[i]
			if pod.Name == state.LeaderName {
				if pod.Labels != nil {
					if raw, ok := pod.Labels[openbaolabels.LabelVersion]; ok {
						v := strings.TrimSpace(raw)
						if v != "" {
							return v
						}
					}
				}
				break
			}
		}
	}

	if state.Pod0 != nil && state.Pod0.Labels != nil {
		if raw, ok := state.Pod0.Labels[openbaolabels.LabelVersion]; ok {
			v := strings.TrimSpace(raw)
			if v != "" {
				return v
			}
		}
	}

	var candidate string
	for i := range state.Pods {
		pod := &state.Pods[i]
		if pod.Labels == nil {
			return ""
		}
		raw, ok := pod.Labels[openbaolabels.LabelVersion]
		if !ok {
			return ""
		}
		v := strings.TrimSpace(raw)
		if v == "" {
			return ""
		}
		if candidate == "" {
			candidate = v
			continue
		}
		if candidate != v {
			return ""
		}
	}
	return candidate
}
