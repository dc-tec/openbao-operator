package openbaocluster

const (
	labelAppName      = "app.kubernetes.io/name"
	labelAppInstance  = "app.kubernetes.io/instance"
	labelAppManagedBy = "app.kubernetes.io/managed-by"

	labelOpenBaoCluster   = "openbao.org/cluster"
	labelOpenBaoComponent = "openbao.org/component"
	labelOpenBaoRevision  = "openbao.org/revision"

	labelValueAppNameOpenBao              = "openbao"
	labelValueAppManagedByOpenBaoOperator = "openbao-operator"

	componentBackup = "backup"
)
