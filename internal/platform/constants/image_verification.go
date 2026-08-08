package constants

// Image verification failure policies.
const (
	ImageVerificationFailurePolicyBlock = "Block"
)

// Condition reasons for image verification failures.
const (
	ReasonImageVerificationFailed                  = "ImageVerificationFailed"
	ReasonInitContainerImageVerificationFailed     = "InitContainerImageVerificationFailed"
	ReasonBackupExecutorImageVerificationFailed    = "BackupExecutorImageVerificationFailed"
	ReasonUpgradeExecutorImageVerificationFailed   = "UpgradeExecutorImageVerificationFailed"
	ReasonRestoreExecutorImageVerificationFailed   = "RestoreExecutorImageVerificationFailed"
	ReasonBlueGreenImageVerificationFailed         = "BlueGreenImageVerificationFailed"
	ReasonBlueGreenSnapshotImageVerificationFailed = "BlueGreenSnapshotImageVerificationFailed"
	ReasonValidationHookImageVerificationFailed    = "ValidationHookImageVerificationFailed"
	ReasonPreUpgradeBackupImageVerificationFailed  = "PreUpgradeBackupImageVerificationFailed"
	ReasonHelperImageConfigurationInvalid          = "HelperImageConfigurationInvalid"
)
