package bluegreen

// Condition reason strings for Blue/Green upgrades.
const (
	// ReasonUpgradeStarted indicates an upgrade has been initiated.
	ReasonUpgradeStarted = "UpgradeStarted"

	// ReasonUpgradeComplete indicates an upgrade has finished successfully.
	ReasonUpgradeComplete = "UpgradeComplete"

	// ReasonUpgradeFailed indicates an upgrade has failed.
	ReasonUpgradeFailed = "UpgradeFailed"

	// AnnotationForceRollback is a manual escape hatch annotation that
	// forces a rollback of the current blue/green upgrade when set to "true".
	// This allows operators to break glass if the state machine becomes stuck.
	AnnotationForceRollback = "openbao.org/force-rollback"
)
