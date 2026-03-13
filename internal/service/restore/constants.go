package restore

// Reason constants for OpenBaoRestore conditions.
const (
	// ReasonRestoreValidationStarted indicates restore validation has started.
	ReasonRestoreValidationStarted = "RestoreValidationStarted"

	// RestoreConfigurationConditionType reports whether operator-known restore
	// prerequisites such as auth references, storage credential references, and
	// hardened-profile egress requirements are satisfied.
	RestoreConfigurationConditionType = "RestoreConfigurationReady"

	// ReasonRestoreStarted indicates restore execution has started.
	ReasonRestoreStarted = "RestoreStarted"

	// ReasonRestoreIdentityConfiguration describes how the generated restore workload receives cloud identity.
	ReasonRestoreIdentityConfiguration = "RestoreIdentityConfiguration"

	// ReasonRestoreJobCreated indicates the restore Job was created successfully.
	ReasonRestoreJobCreated = "RestoreJobCreated"

	// ReasonRestoreFailed indicates the restore operation failed.
	ReasonRestoreFailed = "RestoreFailed"

	// ReasonRestoreCompleted indicates the restore operation completed successfully.
	ReasonRestoreCompleted = "RestoreCompleted"

	// ReasonRestoreSucceeded indicates the restore operation succeeded.
	ReasonRestoreSucceeded = "RestoreSucceeded"

	// ReasonAuthRequired indicates authentication was not configured for restore.
	ReasonAuthRequired = "AuthenticationRequired"

	// ReasonOperationLockBlocked indicates restore is waiting for another operation to release the cluster lock.
	ReasonOperationLockBlocked = "OperationLockBlocked"

	// ReasonOperationLockLost indicates restore lost the cluster lock while running.
	ReasonOperationLockLost = "OperationLockLost"

	// ComponentRestore is the component name for restore resources.
	ComponentRestore = "restore"
)
