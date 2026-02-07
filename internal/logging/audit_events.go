package logging

const (
	// Admission startup events.
	EventAdmissionUnsafeModeEnabled    = "AdmissionUnsafeModeEnabled"
	EventAdmissionDependenciesReady    = "AdmissionDependenciesReady"
	EventAdmissionDependenciesNotReady = "AdmissionDependenciesNotReady"
	EventAdmissionStartupBlocked       = "AdmissionStartupBlocked"
	EventAdmissionCanaryPassed         = "AdmissionCanaryPassed"
	EventAdmissionCanaryFailed         = "AdmissionCanaryFailed"

	// Cross-controller operation lock lifecycle events.
	EventOperationLockAcquired      = "OperationLockAcquired"
	EventOperationLockBlocked       = "OperationLockBlocked"
	EventOperationLockForceAcquired = "OperationLockForceAcquired"
	EventOperationLockReleased      = "OperationLockReleased"

	// Backup lifecycle events.
	EventBackupManualTriggerDetected = "BackupManualTriggerDetected"
	EventBackupManualTriggerSkipped  = "BackupManualTriggerSkipped"
	EventBackupJobCreated            = "BackupJobCreated"
	EventBackupJobSucceeded          = "BackupJobSucceeded"
	EventBackupJobFailed             = "BackupJobFailed"

	// Restore lifecycle events.
	EventRestorePhaseTransition = "RestorePhaseTransition"
	EventRestoreJobCreated      = "RestoreJobCreated"
	EventRestoreCompleted       = "RestoreCompleted"
	EventRestoreFailed          = "RestoreFailed"
	EventRestoreLockLost        = "RestoreLockLost"

	// Upgrade lifecycle events.
	EventUpgradeStarted               = "UpgradeStarted"
	EventUpgradeCompleted             = "UpgradeCompleted"
	EventUpgradeFailed                = "UpgradeFailed"
	EventPreUpgradeSnapshotJobCreated = "PreUpgradeSnapshotJobCreated"
	EventPreUpgradeSnapshotCompleted  = "PreUpgradeSnapshotCompleted"
	EventPreUpgradeSnapshotRetry      = "PreUpgradeSnapshotRetry"
	EventPreUpgradeSnapshotFailed     = "PreUpgradeSnapshotFailed"
	EventBlueGreenPhaseTransition     = "BlueGreenPhaseTransition"
	EventRollbackInitiated            = "RollbackInitiated"
	EventRollbackCompleted            = "RollbackCompleted"
	EventBreakGlassEntered            = "BreakGlassEntered"
	EventBreakGlassAcknowledged       = "BreakGlassAcknowledged"
)
