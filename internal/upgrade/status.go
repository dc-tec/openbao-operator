package upgrade

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// SetUpgradeStarted initializes the upgrade progress status when an upgrade begins.
// This sets up Status.Upgrade with the initial state and updates conditions.
func SetUpgradeStarted(status *openbaov1alpha1.OpenBaoClusterStatus, from, to string, replicas int32, generation int64) {
	now := metav1.Now()

	status.Upgrade = &openbaov1alpha1.UpgradeProgress{
		TargetVersion:    to,
		FromVersion:      from,
		StartedAt:        &now,
		CurrentPartition: replicas, // Start with partition = replicas (no pods updated yet)
		CompletedPods:    []int32{},
	}

	status.Phase = openbaov1alpha1.ClusterPhaseUpgrading

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeStarted,
		Message:            fmt.Sprintf(MessageUpgradeStarted, from, to),
	})

	// Clear any previous degraded condition related to upgrades
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeStarted,
		Message:            "Upgrade in progress",
	})
}

// SetUpgradeProgress updates the upgrade progress during the rolling update.
// This is called after each pod is successfully upgraded.
func SetUpgradeProgress(status *openbaov1alpha1.OpenBaoClusterStatus, partition int32, completedPod int32, totalReplicas int32, generation int64) {
	if status.Upgrade == nil {
		return
	}

	now := metav1.Now()

	status.Upgrade.CurrentPartition = partition
	status.Upgrade.CompletedPods = append(status.Upgrade.CompletedPods, completedPod)

	completedCount := len(status.Upgrade.CompletedPods)

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeInProgress,
		Message:            fmt.Sprintf(MessageUpgradeInProgress, completedCount, totalReplicas, partition),
	})
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
// This clears Status.Upgrade and updates the version and conditions.
func SetUpgradeComplete(status *openbaov1alpha1.OpenBaoClusterStatus, version string, generation int64) {
	now := metav1.Now()

	var fromVersion string
	if status.Upgrade != nil {
		fromVersion = status.Upgrade.FromVersion
	}

	// Clear upgrade progress
	status.Upgrade = nil

	// Update current version
	status.CurrentVersion = version

	// Restore phase to Running
	status.Phase = openbaov1alpha1.ClusterPhaseRunning

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradeComplete,
		Message:            fmt.Sprintf(MessageUpgradeComplete, fromVersion, version),
	})
}

// SetUpgradeFailed marks an upgrade as failed and sets degraded conditions.
// The upgrade progress is preserved to allow resume or manual intervention.
func SetUpgradeFailed(status *openbaov1alpha1.OpenBaoClusterStatus, reason, message string, generation int64) {
	now := metav1.Now()

	// Keep upgrade progress so state can be inspected or resumed
	// Do NOT clear status.Upgrade

	// Set phase to Failed
	status.Phase = openbaov1alpha1.ClusterPhaseFailed

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})
}

// SetUpgradePaused marks an upgrade as paused (due to spec.paused = true).
// The upgrade progress is preserved to allow resume when unpaused.
func SetUpgradePaused(status *openbaov1alpha1.OpenBaoClusterStatus, generation int64) {
	now := metav1.Now()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonUpgradePaused,
		Message:            "Upgrade paused; set spec.paused=false to resume",
	})
}

// ClearUpgrade clears the upgrade status without marking it complete.
// Used when an upgrade needs to be abandoned (e.g., spec.version changed mid-upgrade).
func ClearUpgrade(status *openbaov1alpha1.OpenBaoClusterStatus, reason, message string, generation int64) {
	now := metav1.Now()

	status.Upgrade = nil
	status.Phase = openbaov1alpha1.ClusterPhaseRunning

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionUpgrading),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})
}

// SetDowngradeBlocked sets the degraded condition when a downgrade is attempted.
func SetDowngradeBlocked(status *openbaov1alpha1.OpenBaoClusterStatus, from, to string, generation int64) {
	now := metav1.Now()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonDowngradeBlocked,
		Message:            fmt.Sprintf(MessageDowngradeBlocked, from, to),
	})
}

// SetInvalidVersion sets the degraded condition when a version is invalid.
func SetInvalidVersion(status *openbaov1alpha1.OpenBaoClusterStatus, version string, err error, generation int64) {
	now := metav1.Now()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonInvalidVersion,
		Message:            fmt.Sprintf(MessageInvalidVersion, version, err),
	})
}

// SetClusterNotReady sets the degraded condition when the cluster is not ready for upgrade.
func SetClusterNotReady(status *openbaov1alpha1.OpenBaoClusterStatus, reason string, generation int64) {
	now := metav1.Now()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonClusterNotReady,
		Message:            fmt.Sprintf(MessageClusterNotReady, reason),
	})
}

// SetPreUpgradeBackupFailed sets the degraded condition when a pre-upgrade backup fails.
func SetPreUpgradeBackupFailed(status *openbaov1alpha1.OpenBaoClusterStatus, message string, generation int64) {
	now := metav1.Now()

	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: generation,
		LastTransitionTime: now,
		Reason:             ReasonPreUpgradeBackupFailed,
		Message:            fmt.Sprintf("Pre-upgrade backup failed: %s", message),
	})
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
