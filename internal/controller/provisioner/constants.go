package provisioner

import "github.com/kubebao/openbao-operator/internal/platform/constants"

// Reason constants for Provisioner conditions.
const (
	ReasonSecurityViolation            = constants.ReasonSecurityViolation
	ReasonTenantSecretRBACSynchronized = constants.ReasonTenantSecretRBACSynchronized

	controllerNameNamespaceProvisioner = "namespace-provisioner"
	controllerNameTenantSecretsRBAC    = controllerNameNamespaceProvisioner + "-tenant-secrets"
	conditionTypeProvisioned           = constants.TenantProvisionedConditionType
)
