package restore

// Reason constants for OpenBaoRestore conditions.
const (
	// ReasonRestoreFailed indicates the restore operation failed.
	ReasonRestoreFailed = "RestoreFailed"

	// ReasonRestoreSucceeded indicates the restore operation succeeded.
	ReasonRestoreSucceeded = "RestoreSucceeded"

	// ComponentRestore is the component name for restore resources.
	ComponentRestore = "restore"
)
