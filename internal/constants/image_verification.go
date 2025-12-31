package constants

// Image verification failure policies.
const (
	ImageVerificationFailurePolicyWarn  = "Warn"
	ImageVerificationFailurePolicyBlock = "Block"
)

// Condition reasons for image verification failures.
const (
	ReasonImageVerificationFailed                  = "ImageVerificationFailed"
	ReasonSentinelImageVerificationFailed          = "SentinelImageVerificationFailed"
	ReasonInitContainerImageVerificationFailed     = "InitContainerImageVerificationFailed"
	ReasonBackupExecutorImageVerificationFailed    = "BackupExecutorImageVerificationFailed"
	ReasonUpgradeExecutorImageVerificationFailed   = "UpgradeExecutorImageVerificationFailed"
	ReasonRestoreExecutorImageVerificationFailed   = "RestoreExecutorImageVerificationFailed"
	ReasonBlueGreenImageVerificationFailed         = "BlueGreenImageVerificationFailed"
	ReasonBlueGreenSnapshotImageVerificationFailed = "BlueGreenSnapshotImageVerificationFailed"
	ReasonPreUpgradeBackupImageVerificationFailed  = "PreUpgradeBackupImageVerificationFailed"
)
