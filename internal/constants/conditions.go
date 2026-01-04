package constants

// Common condition reasons used by the operator for various Status conditions.
const (
	// Ready indicates a resource is fully prepared and functional.
	ReasonReady = "Ready"
	// Error indicates a generic failure state.
	ReasonError = "Error"
	// Paused indicates reconciliation is disabled for the resource.
	ReasonPaused = "Paused"

	// Reconciling indicates the resource is currently being reconciled.
	ReasonReconciling = "Reconciling"
	// Idle indicates the resource is in an idle state.
	ReasonIdle = "Idle"
	// Unknown indicates an unknown state.
	ReasonUnknown = "Unknown"
	// ReasonBreakGlassRequired indicates the operator has halted automation and requires manual intervention.
	ReasonBreakGlassRequired = "BreakGlassRequired"

	// ReasonNetworkEgressRulesRequired indicates the cluster requires explicit NetworkPolicy egress rules
	// to proceed with an operation (e.g. backup/restore jobs in Hardened profile).
	ReasonNetworkEgressRulesRequired = "NetworkEgressRulesRequired"
)

const (
	// RestoreConditionType is the condition type for restore operations.
	RestoreConditionType = "RestoreComplete"

	// ConditionTypeProvisioned is the condition type for tenant provisioning.
	ConditionTypeProvisioned = "Provisioned"

	// ConditionTypeOperationLockOverride is the condition type used when an operation
	// lock is forcefully overridden (e.g., during disaster recovery restore).
	ConditionTypeOperationLockOverride = "OperationLockOverride"

	// ReasonOperationLockOverridden indicates that an existing operation lock was
	// cleared to allow a higher-priority operation to proceed.
	ReasonOperationLockOverridden = "OperationLockOverridden"
)
