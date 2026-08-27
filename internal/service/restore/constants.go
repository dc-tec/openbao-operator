package restore

import "github.com/dc-tec/openbao-operator/internal/platform/constants"

// Reason constants for OpenBaoRestore conditions.
const (
	// ReasonRestoreValidationStarted indicates restore validation has started.
	ReasonRestoreValidationStarted = "RestoreValidationStarted"

	// RestoreConfigurationConditionType reports whether operator-known restore
	// prerequisites such as auth references, storage credential references, and
	// hardened-profile egress requirements are satisfied.
	RestoreConfigurationConditionType = constants.RestoreConfigurationConditionType

	// ReasonRestoreStarted indicates restore execution has started.
	ReasonRestoreStarted = "RestoreStarted"

	// ReasonRestoreIdentityConfiguration describes how the generated restore workload receives cloud identity.
	ReasonRestoreIdentityConfiguration = "RestoreIdentityConfiguration"

	// ReasonRestoreJobCreated indicates the restore Job was created successfully.
	ReasonRestoreJobCreated = "RestoreJobCreated"

	// ReasonRestoreExecutionUnknown indicates the controller cannot prove whether
	// a committed restore Job ran and will not recreate it automatically.
	ReasonRestoreExecutionUnknown = "RestoreExecutionUnknown"

	// ReasonRestoreFailed indicates the restore operation failed.
	ReasonRestoreFailed = "RestoreFailed"

	// ReasonRestoreCompleted indicates the restore operation completed successfully.
	ReasonRestoreCompleted = "RestoreCompleted"

	// ReasonRestoreRestartRequested indicates the voter workload restart was requested after snapshot application.
	ReasonRestoreRestartRequested = "RestoreRestartRequested"

	// ReasonRestoreRestartCompleted indicates all voter Pods completed the post-restore restart.
	ReasonRestoreRestartCompleted = "RestoreRestartCompleted"

	// ReasonRestoreSucceeded indicates the restore operation succeeded.
	ReasonRestoreSucceeded = "RestoreSucceeded"

	// ReasonAuthRequired indicates authentication was not configured for restore.
	ReasonAuthRequired = constants.ReasonAuthenticationRequired

	// ReasonOperationLockBlocked indicates restore is waiting for another operation to release the cluster lock.
	ReasonOperationLockBlocked = constants.ReasonOperationLockBlocked

	// ReasonOperationLockLost indicates restore lost the cluster lock while running.
	ReasonOperationLockLost = "OperationLockLost"

	// ReasonOperationLockOverride indicates a force override cleared an existing operation lock.
	ReasonOperationLockOverride = "OperationLockOverride"

	// ComponentRestore is the component name for restore resources.
	ComponentRestore = "restore"
)
