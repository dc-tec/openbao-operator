package constants

// Common Kubernetes label keys used by the operator.
const (
	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppManagedBy = "app.kubernetes.io/managed-by"
	LabelAppComponent = "app.kubernetes.io/component"

	LabelOpenBaoCluster                = "openbao.org/cluster"
	LabelOpenBaoComponent              = "openbao.org/component"
	LabelOpenBaoBackupType             = "openbao.org/backup-type"
	LabelOpenBaoTenant                 = "openbao.org/tenant"
	LabelOpenBaoRevision               = "openbao.org/revision"
	LabelOpenBaoWorkloadPool           = "openbao.org/workload-pool"
	LabelOpenBaoProfile                = "openbao.org/profile"
	LabelOpenBaoCredentialPurpose      = "openbao.org/credential-purpose"
	LabelOpenBaoServiceAccountRole     = "openbao.org/service-account-role"
	LabelOpenBaoAuditFileStorage       = "openbao.org/audit-file-storage"
	LabelOpenBaoSensitive              = "openbao.org/sensitive"
	LabelOpenBaoOwnershipMode          = "openbao.org/ownership-mode"
	LabelOpenBaoClaimNamespace         = "openbao.org/claim-namespace"
	LabelOpenBaoClaimName              = "openbao.org/claim-name"
	LabelOpenBaoClaimRestoreRequest    = "openbao.org/claim-restore-request"
	LabelOpenBaoServiceOfferingRollout = "openbao.org/service-offering-rollout"
	// LabelOpenBaoDigestEnforcement indicates whether digest-only image refs are required.
	LabelOpenBaoDigestEnforcement = "openbao.org/digest-enforcement"
)

// Common label values used by the operator.
const (
	LabelValueAppNameOpenBao              = "openbao"
	LabelValueAppNameOpenBaoOperator      = "openbao-operator"
	LabelValueAppManagedByOpenBaoOperator = "openbao-operator"
	LabelValueDigestEnforcementRequired   = "required"

	LabelValueOpenBaoTenant = "true"

	// Component label values for operator pods.
	LabelValueAppComponentController  = "controller"
	LabelValueAppComponentProvisioner = "provisioner"

	LabelValueOpenBaoWorkloadPoolVoter       = "voter"
	LabelValueOpenBaoWorkloadPoolReadReplica = "read-replica"
	LabelValueCredentialPurposeRestoreToken  = "restore-token"
	LabelValueSensitiveAudit                 = "audit"
	LabelValueOpenBaoOwnershipClaimManaged   = "ClaimManaged"
	LabelValueOpenBaoOwnershipDirectManaged  = "DirectManaged"
)

// ServiceAccount role label values for operator-managed ServiceAccounts.
const (
	ServiceAccountRoleMain    = "main"
	ServiceAccountRoleBackup  = "backup"
	ServiceAccountRoleRestore = "restore"
	ServiceAccountRoleUpgrade = "upgrade"
)

// Backup type values for the openbao.org/backup-type label.
const (
	// BackupTypePreUpgrade indicates a backup taken before an upgrade operation.
	BackupTypePreUpgrade = "pre-upgrade"
	// BackupTypeScheduled indicates a backup taken on a scheduled basis.
	BackupTypeScheduled = "scheduled"
	// BackupTypeManual indicates a manually triggered backup.
	BackupTypeManual = "manual"
)
