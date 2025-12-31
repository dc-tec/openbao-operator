package upgrade

import "time"

// Default timeouts and intervals for upgrade operations.
const (
	// DefaultPodReadyTimeout is the maximum time to wait for a pod to become ready.
	DefaultPodReadyTimeout = 10 * time.Minute

	// DefaultPodReadyCheckInterval is how often to check pod readiness.
	DefaultPodReadyCheckInterval = 5 * time.Second

	// DefaultStepDownTimeout is the maximum time to wait for a leader step-down.
	DefaultStepDownTimeout = 60 * time.Second

	// DefaultHealthCheckTimeout is the timeout for individual health check requests.
	DefaultHealthCheckTimeout = 5 * time.Second

	// DefaultHealthCheckInterval is how often to perform health checks.
	DefaultHealthCheckInterval = 2 * time.Second

	// DefaultLeaderCheckInterval is how often to check for a leader.
	DefaultLeaderCheckInterval = 5 * time.Second

	// ComponentUpgrade is the component name for upgrade resources.
	ComponentUpgrade = "upgrade"
)

// Reason constants for condition updates.
// These are used to set the Reason field in Kubernetes Conditions.
const (
	// ReasonUpgradeStarted indicates the upgrade process has begun.
	ReasonUpgradeStarted = "UpgradeStarted"

	// ReasonUpgradeInProgress indicates the rolling update is in progress.
	ReasonUpgradeInProgress = "UpgradeInProgress"

	// ReasonUpgradeComplete indicates the upgrade finished successfully.
	ReasonUpgradeComplete = "UpgradeComplete"

	// ReasonUpgradeFailed indicates the upgrade process failed.
	ReasonUpgradeFailed = "UpgradeFailed"

	// ReasonUpgradePaused indicates the upgrade was paused.
	ReasonUpgradePaused = "UpgradePaused"

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

	// ReasonNoUpgradeNeeded indicates spec.version matches status.currentVersion.
	ReasonNoUpgradeNeeded = "NoUpgradeNeeded"

	// ReasonVersionMismatch indicates spec.version changed during an upgrade.
	ReasonVersionMismatch = "VersionMismatch"

	// ReasonInvalidVersion indicates the target version is invalid.
	ReasonInvalidVersion = "InvalidVersion"

	// ReasonDowngradeBlocked indicates a downgrade was attempted but blocked.
	ReasonDowngradeBlocked = "DowngradeBlocked"

	// ReasonClusterNotReady indicates the cluster is not in a healthy state for upgrade.
	ReasonClusterNotReady = "ClusterNotReady"
)

// Message constants for condition updates.
const (
	MessageUpgradeStarted           = "Upgrade from %s to %s has started"
	MessageUpgradeInProgress        = "Rolling update in progress: %d/%d replicas updated (partition: %d)"
	MessageUpgradeComplete          = "Upgrade from %s to %s finished successfully"
	MessageUpgradeFailed            = "Upgrade failed: %s"
	MessageUpgradePaused            = "Upgrade paused at partition %d"
	MessageUpgradeResumed           = "Upgrade resumed at partition %d"
	MessageStepDownTimeout          = "Leader step-down timed out for pod %s"
	MessagePodNotReady              = "Pod %s failed to become ready within %v"
	MessageHealthCheckFailed        = "OpenBao health check failed for pod %s: %s"
	MessagePreUpgradeBackupStarted  = "Pre-upgrade backup started"
	MessagePreUpgradeBackupComplete = "Pre-upgrade backup finished successfully"
	MessageDowngradeBlocked         = "Downgrade from %s to %s is not supported"
	MessageInvalidVersion           = "Invalid target version %q: %v"
	MessageClusterNotReady          = "Cluster is not ready for upgrade: %s"
)

// ExecutorAction selects which upgrade operation the upgrade executor performs.
type ExecutorAction string

const (
	ExecutorActionBlueGreenJoinGreenNonVoters          ExecutorAction = "bluegreen-join-green-nonvoters"
	ExecutorActionBlueGreenWaitGreenSynced             ExecutorAction = "bluegreen-wait-green-synced"
	ExecutorActionBlueGreenPromoteGreenVoters          ExecutorAction = "bluegreen-promote-green-voters"
	ExecutorActionBlueGreenDemoteBlueNonVotersStepDown ExecutorAction = "bluegreen-demote-blue-nonvoters-stepdown"
	ExecutorActionBlueGreenRemoveBluePeers             ExecutorAction = "bluegreen-remove-blue-peers"

	// ExecutorActionBlueGreenRepairConsensus repairs Raft consensus during rollback by
	// ensuring Blue nodes are voters and Green nodes are non-voters in a single pass.
	ExecutorActionBlueGreenRepairConsensus ExecutorAction = "bluegreen-repair-consensus"

	ExecutorActionRollingStepDownLeader ExecutorAction = "rolling-stepdown-leader"
)
