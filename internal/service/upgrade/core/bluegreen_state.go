package core

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

// CurrentBlueGreenPhase returns the current blue/green phase, defaulting to Idle.
func CurrentBlueGreenPhase(cluster *openbaov1alpha1.OpenBaoCluster) openbaov1alpha1.BlueGreenPhase {
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return openbaov1alpha1.PhaseIdle
	}
	return cluster.Status.BlueGreen.Phase
}

// IsBlueGreenRollbackSet reports whether rollback tracking has started.
func IsBlueGreenRollbackSet(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Status.BlueGreen != nil &&
		cluster.Status.BlueGreen.RollbackStartTime != nil
}

// BlueGreenUpgradeState reports whether a blue/green upgrade is active and needed.
func BlueGreenUpgradeState(cluster *openbaov1alpha1.OpenBaoCluster) (upgradeActive bool, upgradeNeeded bool) {
	return CurrentBlueGreenPhase(cluster) != openbaov1alpha1.PhaseIdle,
		cluster != nil && cluster.Status.CurrentVersion != "" && cluster.Spec.Version != cluster.Status.CurrentVersion
}

// BlueGreenStartEventPending reports whether the start event should still be emitted.
func BlueGreenStartEventPending(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster == nil ||
		cluster.Status.BlueGreen == nil ||
		cluster.Status.BlueGreen.PreUpgradeSnapshotJobName == ""
}

// InitializeBlueGreenManualPromotion snapshots the current auto-promote mode for a new upgrade.
func InitializeBlueGreenManualPromotion(cluster *openbaov1alpha1.OpenBaoCluster) {
	if cluster == nil ||
		cluster.Status.BlueGreen == nil ||
		cluster.Spec.Upgrade == nil ||
		cluster.Status.BlueGreen.GreenRevision != "" ||
		cluster.Status.BlueGreen.PreUpgradeSnapshotJobName != "" {
		return
	}

	cluster.Status.BlueGreen.ManualPromotionRequired = cluster.Spec.Upgrade.BlueGreen != nil &&
		!cluster.Spec.Upgrade.BlueGreen.AutoPromote
}

// ResetBlueGreenTransientState clears in-flight blue/green fields after a terminal transition.
func ResetBlueGreenTransientState(status *openbaov1alpha1.BlueGreenStatus) {
	if status == nil {
		return
	}
	status.Phase = openbaov1alpha1.PhaseIdle
	status.GreenRevision = ""
	status.ManualPromotionRequired = false
	status.StartTime = nil
	status.JobFailureCount = 0
	status.LastJobFailure = ""
}

// FinalizeBlueGreenTerminalState applies the shared status transition for a
// completed or aborted blue/green flow.
func FinalizeBlueGreenTerminalState(cluster *openbaov1alpha1.OpenBaoCluster, promoteGreenToBlue bool) {
	if cluster == nil || cluster.Status.BlueGreen == nil {
		return
	}

	if promoteGreenToBlue {
		cluster.Status.BlueGreen.BlueRevision = cluster.Status.BlueGreen.GreenRevision
		if cluster.Spec.Image != "" {
			cluster.Status.BlueGreen.BlueImage = cluster.Spec.Image
		}
	}

	ResetBlueGreenTransientState(cluster.Status.BlueGreen)
}
