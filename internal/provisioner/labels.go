package provisioner

const (
	labelAppName      = "app.kubernetes.io/name"
	labelAppComponent = "app.kubernetes.io/component"
	labelAppManagedBy = "app.kubernetes.io/managed-by"

	labelValueAppNameOpenBaoOperator      = "openbao-operator"
	labelValueAppComponentProvisioner     = "provisioner"
	labelValueAppManagedByOpenBaoOperator = "openbao-operator"
)

func provisionerManagedLabels() map[string]string {
	return map[string]string{
		labelAppName:      labelValueAppNameOpenBaoOperator,
		labelAppComponent: labelValueAppComponentProvisioner,
		labelAppManagedBy: labelValueAppManagedByOpenBaoOperator,
	}
}
