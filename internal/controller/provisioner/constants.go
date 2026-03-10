package provisioner

// Reason constants for Provisioner conditions.
const (
	ReasonSecurityViolation            = "SecurityViolation"
	ReasonTenantSecretRBACSynchronized = "TenantSecretRBACSynchronized"

	controllerNameNamespaceProvisioner = "namespace-provisioner"
	controllerNameTenantSecretsRBAC    = controllerNameNamespaceProvisioner + "-tenant-secrets"
	conditionTypeProvisioned           = "Provisioned"
)
