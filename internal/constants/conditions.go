package constants

// Common condition reasons used by the operator for various Status conditions.
const (
	// Error indicates a generic failure state.
	ReasonError = "Error"

	// ReasonNetworkEgressRulesRequired indicates the cluster requires explicit NetworkPolicy egress rules
	// to proceed with an operation (e.g. backup/restore jobs in Hardened profile).
	ReasonNetworkEgressRulesRequired = "NetworkEgressRulesRequired"
)

const (
	// RestoreConditionType is the condition type for restore operations.
	RestoreConditionType = "RestoreComplete"

	// ConditionTypeOperationLockOverride is the condition type used when an operation
	// lock is forcefully overridden (e.g., during disaster recovery restore).
	ConditionTypeOperationLockOverride = "OperationLockOverride"

	// ReasonOperationLockOverridden indicates that an existing operation lock was
	// cleared to allow a higher-priority operation to proceed.
	ReasonOperationLockOverridden = "OperationLockOverridden"
)
