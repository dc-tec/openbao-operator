package bluegreen

// Condition reason strings for Blue/Green upgrades.
const (
	// ReasonUpgradeStarted indicates the blue/green upgrade process has begun.
	ReasonUpgradeStarted = "UpgradeStarted"

	// ReasonUpgradeComplete indicates the blue/green upgrade finished successfully.
	ReasonUpgradeComplete = "UpgradeComplete"

	// ReasonUpgradeFailed indicates the blue/green upgrade process failed.
	ReasonUpgradeFailed = "UpgradeFailed"

	// ReasonUpgradeRollback indicates a blue/green upgrade is being rolled back.
	ReasonUpgradeRollback = "UpgradeRollback"

	// ReasonRollbackFailed indicates a blue/green rollback operation failed.
	ReasonRollbackFailed = "RollbackFailed"

	// AnnotationForceRollback is a manual escape hatch annotation that
	// forces a rollback of the current blue/green upgrade when set to "true".
	// This allows operators to break glass if the state machine becomes stuck.
	AnnotationForceRollback = "openbao.org/force-rollback"

	// AnnotationSnapshotPhase labels snapshot Jobs with their role (e.g. pre-upgrade).
	AnnotationSnapshotPhase = "openbao.org/snapshot-phase"

	// DeploymentNameSuffix is the suffix for the Green StatefulSet name.
	DeploymentNameSuffix = "green"

	// ComponentValidationHook is the component name for validation hook.
	ComponentValidationHook = "validation-hook"

	// ComponentUpgradeSnapshot is the component name for upgrade snapshot.
	ComponentUpgradeSnapshot = "upgrade-snapshot"
)
