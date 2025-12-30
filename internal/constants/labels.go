package constants

// Common Kubernetes label keys used by the operator.
const (
	LabelAppName      = "app.kubernetes.io/name"
	LabelAppInstance  = "app.kubernetes.io/instance"
	LabelAppManagedBy = "app.kubernetes.io/managed-by"
	LabelAppComponent = "app.kubernetes.io/component"

	LabelOpenBaoCluster    = "openbao.org/cluster"
	LabelOpenBaoComponent  = "openbao.org/component"
	LabelOpenBaoBackupType = "openbao.org/backup-type"
	LabelOpenBaoTenant     = "openbao.org/tenant"
	LabelOpenBaoRevision   = "openbao.org/revision"
)

// Common label values used by the operator.
const (
	LabelValueAppNameOpenBao              = "openbao"
	LabelValueAppNameOpenBaoOperator      = "openbao-operator"
	LabelValueAppManagedByOpenBaoOperator = "openbao-operator"

	LabelValueOpenBaoTenant = "true"
)
