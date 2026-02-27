package openbaocluster

import (
	"strings"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	controllerdeps "github.com/dc-tec/openbao-operator/internal/controller/openbaocluster/deps"
	openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"
)

func (r *OpenBaoClusterReconciler) reconcileCurrentVersion(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, state *clusterState, observedVersion string) {
	// Initialize (or correct) CurrentVersion from observed running pods.
	//
	// RATIONALE: spec.version can be updated ahead of the actual workload rollout
	// (for example, RollingUpdate with a locked partition). Status must reflect the
	// currently-running OpenBao version to drive safe upgrade orchestration.
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

	// Freeze CurrentVersion correction while either upgrade strategy has status state.
	// This prevents status churn from fighting an in-progress or failed upgrade flow.
	if state.RollingUpgradeInProgress || state.BlueGreenInProgress {
		return
	}

	if observedVersion == "" || cluster.Status.CurrentVersion == "" || cluster.Status.CurrentVersion == observedVersion {
		return
	}

	isDowngrade, err := controllerdeps.IsVersionDowngrade(cluster.Status.CurrentVersion, observedVersion)
	if err != nil {
		logger.V(1).Info("Skipping CurrentVersion correction due to unparsable version",
			"currentVersion", cluster.Status.CurrentVersion,
			"observedVersion", observedVersion,
			"error", err)
		return
	}
	if isDowngrade {
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

func (r *OpenBaoClusterReconciler) maybeAdvanceCurrentVersionForBlueGreen(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, observedVersion string) {
	// Detect BlueGreen upgrade completion: if BlueGreen is Idle and CurrentVersion
	// doesn't match Spec.Version, the upgrade completed and we should update.
	// This allows Status controller to own currentVersion while BlueGreen manager
	// signals completion via phase transition.
	if cluster.Status.BlueGreen == nil ||
		cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle ||
		cluster.Status.CurrentVersion == "" ||
		cluster.Status.CurrentVersion == cluster.Spec.Version ||
		cluster.Spec.Upgrade == nil ||
		cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen {
		return
	}

	// CRITICAL CHECK: Verify that the upgrade actually happened.
	// PhaseIdle can mean "Before Upgrade" OR "After Upgrade".
	// We distinguish them by checking if the active BlueRevision matches the current Spec.
	currentSpecRevision := controllerdeps.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
	if cluster.Status.BlueGreen.BlueRevision != currentSpecRevision {
		return
	}

	// Extra safety: only advance the version if pods are actually reporting the target version.
	// If the label is missing, fall back to the revision-based check above.
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

func (r *OpenBaoClusterReconciler) warnIfSelfInitDisabled(logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	// SECURITY: Warn when SelfInit is disabled - the operator will store the root token.
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if selfInitEnabled {
		return
	}

	logger.Info("SECURITY WARNING: SelfInit is disabled - root token will be stored in Secret",
		"cluster_namespace", cluster.Namespace,
		"cluster_name", cluster.Name,
		"secret_name", cluster.Name+"-root-token")
}

func observedVersionFromPods(state *clusterState) string {
	if state == nil || len(state.Pods) == 0 {
		return ""
	}

	// Prefer the leader's reported version only when leadership is unambiguous.
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

	// Next, prefer pod0 (stable identity).
	if state.Pod0 != nil && state.Pod0.Labels != nil {
		if raw, ok := state.Pod0.Labels[openbaolabels.LabelVersion]; ok {
			v := strings.TrimSpace(raw)
			if v != "" {
				return v
			}
		}
	}

	// Finally, if all pods report the same non-empty version, use it.
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
