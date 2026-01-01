package upgrade

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// SetUpgradeStarted initializes the upgrade progress status when an upgrade begins.
func SetUpgradeStarted(status *openbaov1alpha1.OpenBaoClusterStatus, from, to string, replicas int32, generation int64) {
	now := metav1.Now()

	status.Upgrade = &openbaov1alpha1.UpgradeProgress{
		TargetVersion:    to,
		FromVersion:      from,
		StartedAt:        &now,
		CurrentPartition: replicas, // Start with partition = replicas (no pods updated yet)
		CompletedPods:    []int32{},
		LastErrorReason:  "",
		LastErrorMessage: "",
		LastErrorAt:      nil,
	}
}

// SetUpgradeProgress updates the upgrade progress during the rolling update.
// This is called after each pod is successfully upgraded.
func SetUpgradeProgress(status *openbaov1alpha1.OpenBaoClusterStatus, partition int32, completedPod int32, totalReplicas int32, generation int64) {
	if status.Upgrade == nil {
		return
	}

	status.Upgrade.CurrentPartition = partition
	status.Upgrade.CompletedPods = append(status.Upgrade.CompletedPods, completedPod)
}

// SetStepDownPerformed records that a leader step-down was performed.
func SetStepDownPerformed(status *openbaov1alpha1.OpenBaoClusterStatus) {
	if status.Upgrade == nil {
		return
	}

	now := metav1.Now()
	status.Upgrade.LastStepDownTime = &now
}

// SetUpgradeComplete finalizes the upgrade status after successful completion.
// This clears Status.Upgrade and updates the version.
func SetUpgradeComplete(status *openbaov1alpha1.OpenBaoClusterStatus, version string, generation int64) {
	// Clear upgrade progress
	status.Upgrade = nil

	// Update current version
	status.CurrentVersion = version
}

// SetUpgradeFailed marks an upgrade as failed and sets degraded conditions.
// The upgrade progress is preserved to allow resume or manual intervention.
func SetUpgradeFailed(status *openbaov1alpha1.OpenBaoClusterStatus, reason, message string, generation int64) {
	now := metav1.Now()

	// Keep upgrade progress so state can be inspected or resumed
	// Do NOT clear status.Upgrade
	if status.Upgrade == nil {
		status.Upgrade = &openbaov1alpha1.UpgradeProgress{}
	}
	status.Upgrade.LastErrorReason = reason
	status.Upgrade.LastErrorMessage = message
	status.Upgrade.LastErrorAt = &now
}

// ClearUpgrade clears the upgrade status without marking it complete.
// Used when an upgrade needs to be abandoned (e.g., spec.version changed mid-upgrade).
func ClearUpgrade(status *openbaov1alpha1.OpenBaoClusterStatus, reason, message string, generation int64) {
	status.Upgrade = nil
}

// IsUpgradeInProgress returns true if an upgrade is currently in progress.
func IsUpgradeInProgress(status *openbaov1alpha1.OpenBaoClusterStatus) bool {
	return status.Upgrade != nil
}

// GetUpgradeTargetVersion returns the target version of an in-progress upgrade.
// Returns empty string if no upgrade is in progress.
func GetUpgradeTargetVersion(status *openbaov1alpha1.OpenBaoClusterStatus) string {
	if status.Upgrade == nil {
		return ""
	}
	return status.Upgrade.TargetVersion
}
