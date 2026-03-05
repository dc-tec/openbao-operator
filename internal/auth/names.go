package auth

// JWT auth role names used by the operator and helper executors.
const (
	RoleNameOperator = "openbao-operator"
	RoleNameBackup   = "openbao-operator-backup"
	RoleNameUpgrade  = "openbao-operator-upgrade"
	RoleNameRestore  = "openbao-operator-restore"
)

// Policy names used by the operator and helper executors.
const (
	PolicyNameOperator = "openbao-operator"
	PolicyNameBackup   = "openbao-operator-backup"
	PolicyNameUpgrade  = "openbao-operator-upgrade"
	PolicyNameRestore  = "openbao-operator-restore"
)
