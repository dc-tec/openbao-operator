package provisioner

import "github.com/dc-tec/openbao-operator/internal/platform/constants"

// Reason constants for Provisioner conditions.
const (
	ReasonSecurityViolation            = constants.ReasonSecurityViolation
	ReasonTenantSecretRBACSynchronized = constants.ReasonTenantSecretRBACSynchronized

	controllerNameNamespaceProvisioner = "namespace-provisioner"
	controllerNameTenantSecretsRBAC    = controllerNameNamespaceProvisioner + "-tenant-secrets"
	conditionTypeProvisioned           = constants.TenantProvisionedConditionType
)
