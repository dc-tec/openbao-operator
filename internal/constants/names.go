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
	SuffixRestoreServiceAccount = "-restore-serviceaccount"

	PrefixRestoreJob = "restore-"
)

// Component names used for labeling and observability.
const (
	ComponentOpenBaoCluster  = "openbaocluster"
	ComponentBackup          = "backup"
	ComponentRestore         = "restore"
	ComponentSentinel        = "sentinel"
	ComponentUpgradeSnapshot = "upgrade-snapshot"
	ComponentProvisioner     = "provisioner"
	ComponentController      = "controller"
)

// Well-known container and binary names used across the operator and helper binaries.
const (
	// ContainerBao is the name of the OpenBao container.
	ContainerBao = "openbao"
	// BinaryBao is the name of the OpenBao binary.
	BinaryBao = "bao"
)

// Controller names for observability.
const (
	ControllerNameOpenBaoCluster         = "openbaocluster"
	ControllerNameOpenBaoClusterStatus   = "openbaocluster-status"
	ControllerNameOpenBaoClusterWorkload = "openbaocluster-workload"
	ControllerNameOpenBaoClusterAdminOps = "openbaocluster-adminops"
	ControllerNameOpenBaoRestore         = "openbaorestore"
	ControllerNameNamespaceProvisioner   = "namespace-provisioner"
)
