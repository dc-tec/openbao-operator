package config

const (
	// OpenBao API Paths
	pathSysAuthJWT           = "sys/auth/jwt-operator"
	pathAuthJWTConfig        = "auth/jwt-operator/config"
	pathSysPoliciesACLPrefix = "sys/policies/acl/"
	pathAuthJWTRolePrefix    = "auth/jwt-operator/role/"

	// Operations
	opUpdate = "update"

	// Request Names
	reqEnableJWTAuth        = "enable-jwt-auth"
	reqConfigJWTAuth        = "config-jwt-auth"
	reqCreateOperatorPolicy = "create-operator-policy"
	reqCreateOperatorRole   = "create-operator-role"
	reqCreateBackupPolicy   = "create-backup-policy"
	reqCreateBackupRole     = "create-backup-jwt-role"
	reqCreateUpgradePolicy  = "create-upgrade-policy"
	reqCreateUpgradeRole    = "create-upgrade-jwt-role"
	reqCreateRestorePolicy  = "create-restore-policy"
	reqCreateRestoreRole    = "create-restore-jwt-role"

	// Auth Data
	authMethodJWT = "jwt"
	authDesc      = "Auth method for OpenBao Operator"
)
