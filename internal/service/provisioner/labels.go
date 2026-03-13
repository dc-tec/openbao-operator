package provisioner

import "github.com/dc-tec/openbao-operator/internal/platform/constants"

const (
	labelValueOpenBaoComponentProvisioner = "provisioner"
)

func provisionerManagedLabels() map[string]string {
	return map[string]string{
		constants.LabelAppName:          constants.LabelValueAppNameOpenBaoOperator,
		constants.LabelAppComponent:     constants.LabelValueAppComponentProvisioner,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoComponent: labelValueOpenBaoComponentProvisioner,
	}
}
