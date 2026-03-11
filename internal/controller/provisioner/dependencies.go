package provisioner

import (
	appprovisioner "github.com/dc-tec/openbao-operator/internal/app/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (r *NamespaceProvisionerReconciler) tenantRuntime() appprovisioner.TenantRuntime {
	return appprovisioner.TenantRuntime{
		Client:                   r.Client,
		APIReader:                r.APIReader,
		Recorder:                 r.Recorder,
		Provisioner:              r.Provisioner,
		OperatorNamespace:        r.OperatorNamespace,
		ConditionTypeProvisioned: conditionTypeProvisioned,
		RequeueShort:             constants.RequeueShort,
		RequeueStandard:          constants.RequeueStandard,
	}
}
