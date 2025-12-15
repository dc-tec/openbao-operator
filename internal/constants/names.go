package constants

// Resource name suffixes used by the operator when creating per-cluster resources.
const (
	SuffixTLSCA     = "-tls-ca"
	SuffixTLSServer = "-tls-server"

	SuffixConfigMap      = "-config"
	SuffixRootToken      = "-root-token"
	SuffixUnsealKey      = "-unseal-key"
	SuffixServiceAccount = "-serviceaccount"

	SuffixBackupServiceAccount  = "-backup-serviceaccount"
	SuffixUpgradeServiceAccount = "-upgrade-serviceaccount"
)

// Well-known container and binary names used across the operator and helper binaries.
const (
	ContainerNameOpenBao = "openbao"
	BinaryNameOpenBao    = "bao"
)
