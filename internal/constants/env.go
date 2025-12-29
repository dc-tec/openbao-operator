package constants

// Environment variable keys shared between the operator and helper binaries.
const (
	// Kubernetes metadata
	EnvHostname = "HOSTNAME"
	EnvPodIP    = "POD_IP"

	// OpenBao configuration and service registration
	EnvBaoAPIAddr      = "BAO_API_ADDR"
	EnvBaoK8sPodName   = "BAO_K8S_POD_NAME"
	EnvBaoK8sNamespace = "BAO_K8S_NAMESPACE"

	// Backup executor (cluster context)
	EnvClusterNamespace = "CLUSTER_NAMESPACE"
	EnvClusterName      = "CLUSTER_NAME"
	EnvClusterReplicas  = "CLUSTER_REPLICAS"
	EnvStatefulSetName  = "STATEFULSET_NAME" // StatefulSet name for pod discovery (may include revision for Blue/Green)

	// Upgrade executor
	EnvUpgradeAction        = "UPGRADE_ACTION"
	EnvUpgradeJWTAuthRole   = "UPGRADE_JWT_AUTH_ROLE"
	EnvUpgradeBlueRevision  = "UPGRADE_BLUE_REVISION"
	EnvUpgradeGreenRevision = "UPGRADE_GREEN_REVISION"
	EnvUpgradeSyncThreshold = "UPGRADE_SYNC_THRESHOLD"
	EnvUpgradeTimeout       = "UPGRADE_TIMEOUT"

	// Backup executor (S3/object storage target)
	EnvBackupEndpoint       = "BACKUP_ENDPOINT"
	EnvBackupBucket         = "BACKUP_BUCKET"
	EnvBackupPathPrefix     = "BACKUP_PATH_PREFIX"
	EnvBackupFilenamePrefix = "BACKUP_FILENAME_PREFIX"
	EnvBackupRegion         = "BACKUP_REGION"
	EnvBackupUsePathStyle   = "BACKUP_USE_PATH_STYLE"

	EnvBackupPartSize    = "BACKUP_PART_SIZE"
	EnvBackupConcurrency = "BACKUP_CONCURRENCY"

	// Backup executor (credentials + auth)
	EnvBackupAuthMethod  = "BACKUP_AUTH_METHOD"
	EnvBackupJWTAuthRole = "BACKUP_JWT_AUTH_ROLE"

	EnvBackupTokenPath       = "BACKUP_TOKEN_PATH"       // #nosec G101 -- This is an environment variable name constant, not a credential
	EnvBackupCredentialsPath = "BACKUP_CREDENTIALS_PATH" // #nosec G101 -- This is an environment variable name constant, not a credential
	EnvJWTTokenPath          = "JWT_TOKEN_PATH"          // #nosec G101 -- This is an environment variable name constant, not a credential
	EnvTLSCAPath             = "TLS_CA_PATH"

	EnvBackupCredentialsSecretName      = "BACKUP_CREDENTIALS_SECRET_NAME"      // #nosec G101 -- This is an environment variable name constant, not a credential
	EnvBackupCredentialsSecretNamespace = "BACKUP_CREDENTIALS_SECRET_NAMESPACE" // #nosec G101 -- This is an environment variable name constant, not a credential
	EnvBackupTokenSecretName            = "BACKUP_TOKEN_SECRET_NAME"
	EnvBackupTokenSecretNamespace       = "BACKUP_TOKEN_SECRET_NAMESPACE"

	EnvAWSRoleARN              = "AWS_ROLE_ARN"
	EnvAWSWebIdentityTokenFile = "AWS_WEB_IDENTITY_TOKEN_FILE" // #nosec G101 -- This is an environment variable name constant, not a credential

	// Sentinel
	EnvPodNamespace                  = "POD_NAMESPACE"
	EnvSentinelDebounceWindowSeconds = "SENTINEL_DEBOUNCE_WINDOW_SECONDS"
	EnvOperatorVersion               = "OPERATOR_VERSION"
)

// Backup authentication method values.
const (
	BackupAuthMethodJWT   = "jwt"
	BackupAuthMethodToken = "token"
)
