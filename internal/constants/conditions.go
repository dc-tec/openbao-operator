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
)

const (
	// RestoreConditionType is the condition type for restore operations.
	RestoreConditionType = "RestoreComplete"

	// ConditionTypeProvisioned is the condition type for tenant provisioning.
	ConditionTypeProvisioned = "Provisioned"
)
