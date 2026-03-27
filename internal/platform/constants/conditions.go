package constants

// Common condition reasons used by the operator for various Status conditions.
const (
	// Error indicates a generic failure state.
	ReasonError = "Error"
	// ReasonUnknown indicates the operator cannot yet determine a more specific state.
	ReasonUnknown = "Unknown"
	// ReasonLeaderUnknown indicates the operator cannot determine the current cluster leader.
	ReasonLeaderUnknown = "LeaderUnknown"
	// ReasonPrerequisitesMissing indicates a required prerequisite is absent or invalid.
	ReasonPrerequisitesMissing = "PrerequisitesMissing"
	// ReasonAuthenticationRequired indicates an operator-managed auth path is missing.
	ReasonAuthenticationRequired = "AuthenticationRequired"
	// ReasonTokenSecretMissing indicates a referenced token Secret is missing.
	ReasonTokenSecretMissing = "TokenSecretMissing"
	// ReasonCredentialsSecretMissing indicates a referenced storage credentials Secret is missing.
	ReasonCredentialsSecretMissing = "CredentialsSecretMissing"
	// ReasonWorkloadIdentityConfigured indicates the operator can identify an
	// explicit workload-identity path rather than inferring an ambient default chain.
	ReasonWorkloadIdentityConfigured = "WorkloadIdentityConfigured"
	// ReasonAmbientIdentityAssumed indicates the operator is relying on workload identity
	// or the provider default credential chain rather than a static credentials Secret.
	ReasonAmbientIdentityAssumed = "AmbientIdentityAssumed"
	// ReasonSecurityViolation indicates a request or configuration violates an operator guardrail.
	ReasonSecurityViolation = "SecurityViolation"

	// ReasonNetworkEgressRulesRequired indicates the cluster requires explicit NetworkPolicy egress rules
	// to proceed with an operation (e.g. backup/restore jobs in Hardened profile).
	ReasonNetworkEgressRulesRequired = "NetworkEgressRulesRequired"
	// ReasonOperationLockBlocked indicates another workflow currently holds the cluster operation lock.
	ReasonOperationLockBlocked = "OperationLockBlocked"
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
