package bluegreen

import "github.com/dc-tec/openbao-operator/internal/platform/constants"

// Condition reason strings for Blue/Green upgrades.
const (
	// ReasonUpgradeStarted indicates the blue/green upgrade process has begun.
	ReasonUpgradeStarted = constants.ReasonUpgradeStarted

	// ReasonUpgradeComplete indicates the blue/green upgrade finished successfully.
	ReasonUpgradeComplete = constants.ReasonUpgradeComplete

	// ReasonUpgradeFailed indicates the blue/green upgrade process failed.
	ReasonUpgradeFailed = constants.ReasonUpgradeFailed

	// ReasonUpgradeRollback indicates a blue/green upgrade is being rolled back.
	ReasonUpgradeRollback = "UpgradeRollback"

	// ReasonRollbackFailed indicates a blue/green rollback operation failed.
	ReasonRollbackFailed = "RollbackFailed"

	// ReasonBlueGreenHoldEntered indicates the upgrade is waiting for manual promotion approval.
	ReasonBlueGreenHoldEntered = "BlueGreenHoldEntered"

	// ReasonBlueGreenPromotionApproved indicates manual promotion approval was observed.
	ReasonBlueGreenPromotionApproved = "BlueGreenPromotionApproved"

	// ReasonRollbackStarted indicates rollback has started.
	ReasonRollbackStarted = "RollbackStarted"

	// ReasonBreakGlassEntered indicates automation entered break-glass mode.
	ReasonBreakGlassEntered = "BreakGlassEntered"

	// ReasonBreakGlassAcknowledged indicates break-glass mode was acknowledged by an operator.
	ReasonBreakGlassAcknowledged = "BreakGlassAcknowledged"

	// AnnotationSnapshotPhase labels snapshot Jobs with their role (e.g. pre-upgrade).
	AnnotationSnapshotPhase = "openbao.org/snapshot-phase"

	// DeploymentNameSuffix is the suffix for the Green StatefulSet name.
	DeploymentNameSuffix = "green"

	// ComponentValidationHook is the component name for validation hook.
	ComponentValidationHook = "validation-hook"

	// ComponentUpgradeSnapshot is the component name for upgrade snapshot.
	ComponentUpgradeSnapshot = "upgrade-snapshot"
)
