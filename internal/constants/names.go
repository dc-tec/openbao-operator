package constants

// Resource name suffixes used by the operator when creating per-cluster resources.
const (
	SuffixTLSCA     = "-tls-ca"
	SuffixTLSServer = "-tls-server"

	SuffixConfigMap      = "-config"
	SuffixRootToken      = "-root-token"
	SuffixUnsealKey      = "-unseal-key"
	SuffixServiceAccount = "-serviceaccount"

	SuffixBackupServiceAccount    = "-backup-serviceaccount"
	SuffixUpgradeServiceAccount   = "-upgrade-serviceaccount"
	SuffixRestoreServiceAccount   = "-restore-serviceaccount"
	SuffixAutopilotServiceAccount = "-autopilot-serviceaccount"

	PrefixRestoreJob   = "restore-"
	PrefixAutopilotJob = "autopilot-config-"
)

// Component names used for labeling and observability.
const (
	ComponentOpenBaoCluster  = "openbaocluster"
	ComponentBackup          = "backup"
	ComponentRestore         = "restore"
	ComponentUpgradeSnapshot = "upgrade-snapshot"
	ComponentAutopilot       = "autopilot"
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

// JWT Auth Role names.
const (
	// RoleNameOperator is the JWT auth role used by the operator itself (e.g. for autopilot).
	RoleNameOperator = "openbao-operator"
	// RoleNameBackup is the default JWT auth role suffix for backup operations.
	RoleNameBackup = "openbao-operator-backup"
	// RoleNameUpgrade is the default JWT auth role suffix for upgrade operations.
	RoleNameUpgrade = "openbao-operator-upgrade"
	// RoleNameRestore is the default JWT auth role suffix for restore operations.
	RoleNameRestore = "openbao-operator-restore"

	// PolicyNameOperator is the policy name used by the operator itself.
	PolicyNameOperator = "openbao-operator"
	// PolicyNameBackup is the policy name used for backup operations.
	PolicyNameBackup = "openbao-operator-backup"
	// PolicyNameUpgrade is the policy name used for upgrade operations.
	PolicyNameUpgrade = "openbao-operator-upgrade"
	// PolicyNameRestore is the policy name used for restore operations.
	PolicyNameRestore = "openbao-operator-restore"
)
