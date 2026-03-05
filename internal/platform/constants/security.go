package constants

// Stable user/group IDs pinned by container security contexts.
const (
	UserOpenBao  int64 = 100
	GroupOpenBao int64 = 1000

	UserBackup  int64 = 1000
	GroupBackup int64 = 1000

	UserNonRoot int64 = 65532
)
