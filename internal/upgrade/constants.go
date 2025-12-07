package upgrade

import "time"

// Timeout constants for upgrade operations.
// These values provide sensible defaults while allowing upgrades to complete
// even on slower infrastructure or during high load.
const (
	// DefaultStepDownTimeout is the maximum time to wait for a leader step-down
	// to complete and leadership to transfer to another node.
	DefaultStepDownTimeout = 30 * time.Second

	// DefaultPodReadyTimeout is the maximum time to wait for a pod to become
	// Ready after its container has been updated with a new image.
	DefaultPodReadyTimeout = 5 * time.Minute

	// DefaultHealthCheckTimeout is the maximum time to wait for OpenBao to
	// become healthy (initialized and unsealed) after a pod restart.
	DefaultHealthCheckTimeout = 2 * time.Minute

	// DefaultRaftSyncTimeout is the maximum time to wait for a newly updated
	// pod to sync its Raft log with the cluster.
	DefaultRaftSyncTimeout = 2 * time.Minute

	// DefaultHealthCheckInterval is the interval between health check polls
	// when waiting for a pod to become healthy.
	DefaultHealthCheckInterval = 5 * time.Second

	// DefaultLeaderCheckInterval is the interval between checks when waiting
	// for leadership transfer after step-down.
	DefaultLeaderCheckInterval = 2 * time.Second

	// DefaultPodReadyCheckInterval is the interval between checks when waiting
	// for a pod to become Ready.
	DefaultPodReadyCheckInterval = 5 * time.Second
)

// Reason constants for condition updates.
// These are used to set the Reason field in Kubernetes Conditions.
const (
	// ReasonUpgradeStarted indicates an upgrade has been initiated.
	ReasonUpgradeStarted = "UpgradeStarted"

	// ReasonUpgradeInProgress indicates an upgrade is currently running.
	ReasonUpgradeInProgress = "UpgradeInProgress"

	// ReasonUpgradeComplete indicates an upgrade has finished successfully.
	ReasonUpgradeComplete = "UpgradeComplete"

	// ReasonUpgradeFailed indicates an upgrade has failed.
	ReasonUpgradeFailed = "UpgradeFailed"

	// ReasonUpgradePaused indicates an upgrade is paused due to spec.paused.
	ReasonUpgradePaused = "UpgradePaused"

	// ReasonInvalidVersion indicates the target version is not valid semver.
	ReasonInvalidVersion = "InvalidVersion"

	// ReasonDowngradeBlocked indicates a downgrade was attempted but blocked.
	ReasonDowngradeBlocked = "DowngradeBlocked"

	// ReasonClusterNotReady indicates the cluster is not in a state suitable
	// for upgrades (e.g., missing pods, degraded state).
	ReasonClusterNotReady = "ClusterNotReady"

	// ReasonQuorumLost indicates the cluster has lost quorum and upgrades
	// cannot proceed safely.
	ReasonQuorumLost = "QuorumLost"

	// ReasonLeaderUnknown indicates the operator could not determine the
	// cluster leader, possibly due to split-brain or network issues.
	ReasonLeaderUnknown = "LeaderUnknown"

	// ReasonStepDownTimeout indicates a leader step-down operation timed out.
	ReasonStepDownTimeout = "StepDownTimeout"

	// ReasonStepDownFailed indicates a leader step-down operation failed.
	ReasonStepDownFailed = "StepDownFailed"

	// ReasonPodNotReady indicates a pod failed to become ready within timeout.
	ReasonPodNotReady = "PodNotReady"

	// ReasonHealthCheckFailed indicates OpenBao health checks failed.
	ReasonHealthCheckFailed = "HealthCheckFailed"

	// ReasonPreUpgradeBackupFailed indicates the pre-upgrade backup failed.
	ReasonPreUpgradeBackupFailed = "PreUpgradeBackupFailed"

	// ReasonIdle indicates no upgrade activity.
	ReasonIdle = "Idle"

	// ReasonNoUpgradeNeeded indicates spec.version matches status.currentVersion.
	ReasonNoUpgradeNeeded = "NoUpgradeNeeded"

	// ReasonVersionMismatch indicates spec.version changed during an upgrade.
	ReasonVersionMismatch = "VersionMismatch"
)

// Message constants for condition updates.
const (
	// MessageUpgradeStarted is the message when an upgrade begins.
	MessageUpgradeStarted = "Upgrade from %s to %s has started"

	// MessageUpgradeInProgress is the message during an upgrade.
	MessageUpgradeInProgress = "Upgrading pod %d of %d (partition: %d)"

	// MessageUpgradeComplete is the message when an upgrade completes.
	MessageUpgradeComplete = "Successfully upgraded from %s to %s"

	// MessageUpgradeFailed is the message when an upgrade fails.
	MessageUpgradeFailed = "Upgrade failed: %s"

	// MessageNoUpgradeNeeded is the message when no upgrade is required.
	MessageNoUpgradeNeeded = "No upgrade needed; current version matches desired version"

	// MessageDowngradeBlocked is the message when a downgrade is blocked.
	MessageDowngradeBlocked = "Downgrade from %s to %s is not allowed"

	// MessageInvalidVersion is the message when a version is invalid.
	MessageInvalidVersion = "Invalid version %q: %s"

	// MessageClusterNotReady is the message when cluster is not ready.
	MessageClusterNotReady = "Cluster is not ready for upgrade: %s"

	// MessageQuorumLost is the message when quorum is lost.
	MessageQuorumLost = "Cannot proceed with upgrade: cluster has lost quorum (%d/%d healthy)"

	// MessageLeaderUnknown is the message when leader cannot be determined.
	MessageLeaderUnknown = "Cannot determine cluster leader; upgrade halted for safety"

	// MessageStepDownTimeout is the message when step-down times out.
	MessageStepDownTimeout = "Leader step-down timed out after %v"

	// MessagePodNotReady is the message when a pod fails to become ready.
	MessagePodNotReady = "Pod %s did not become ready within %v"

	// MessageHealthCheckFailed is the message when health checks fail.
	MessageHealthCheckFailed = "OpenBao health check failed for pod %s: %s"

	// MessagePreUpgradeBackupFailed is the message when pre-upgrade backup fails.
	MessagePreUpgradeBackupFailed = "Pre-upgrade backup failed: %s"
)
