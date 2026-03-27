package provisioner

import (
	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	ReasonTenantProvisioned            = constants.ReasonTenantProvisioned
	ReasonTenantRBACCleaned            = constants.ReasonTenantRBACCleaned
	ReasonTenantProvisioningBlocked    = constants.ReasonTenantProvisioningBlocked
	ReasonTenantProvisioningFailed     = constants.ReasonTenantProvisioningFailed
	ReasonTenantSecretRBACSynchronized = constants.ReasonTenantSecretRBACSynchronized
)

func (r TenantRuntime) emitTenantNormalEvent(tenant *openbaov1alpha1.OpenBaoTenant, reason, note string) {
	r.emitTenantEvent(tenant, corev1.EventTypeNormal, reason, note)
}

func (r TenantRuntime) emitTenantWarningEvent(tenant *openbaov1alpha1.OpenBaoTenant, reason, note string) {
	r.emitTenantEvent(tenant, corev1.EventTypeWarning, reason, note)
}

func (r TenantRuntime) emitTenantEvent(tenant *openbaov1alpha1.OpenBaoTenant, eventType, reason, note string) {
	if r.Recorder == nil || tenant == nil {
		return
	}
	r.Recorder.Eventf(tenant, nil, eventType, reason, reason, "%s", note)
}
