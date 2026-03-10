package backup

// Reason constants for OpenBaoBackup conditions.
const (
	// ReasonNoBackupToken indicates the backup token is missing.
	ReasonNoBackupToken = "NoBackupToken"

	// ReasonBackupManualTriggerAccepted indicates a manual backup request was accepted.
	ReasonBackupManualTriggerAccepted = "BackupManualTriggerAccepted"

	// ReasonBackupSkipped indicates a due backup was intentionally skipped.
	ReasonBackupSkipped = "BackupSkipped"

	// ReasonBackupStarted indicates the operator has started a backup attempt.
	ReasonBackupStarted = "BackupStarted"

	// ReasonBackupJobCreated indicates the backup Job was created successfully.
	ReasonBackupJobCreated = "BackupJobCreated"

	// ReasonBackupCompleted indicates the backup completed successfully.
	ReasonBackupCompleted = "BackupCompleted"

	// ReasonBackupFailed indicates the backup failed.
	ReasonBackupFailed = "BackupFailed"

	// ReasonOperationLockBlocked indicates backup could not proceed because another operation holds the cluster lock.
	ReasonOperationLockBlocked = "OperationLockBlocked"

	// ComponentBackup is the component name for backup resources.
	ComponentBackup = "backup"
)
