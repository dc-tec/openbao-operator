package bluegreen

// Condition reason strings for Blue/Green upgrades.
const (
	// ReasonUpgradeStarted indicates an upgrade has been initiated.
	ReasonUpgradeStarted = "UpgradeStarted"

	// ReasonUpgradeComplete indicates an upgrade has finished successfully.
	ReasonUpgradeComplete = "UpgradeComplete"

	// ReasonUpgradeFailed indicates an upgrade has failed.
	ReasonUpgradeFailed = "UpgradeFailed"
)
