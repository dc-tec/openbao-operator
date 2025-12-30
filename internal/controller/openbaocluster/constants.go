package openbaocluster

// Reason constants for OpenBaoCluster conditions.
const (
	ReasonImageVerificationFailed         = "ImageVerificationFailed"
	ReasonSentinelImageVerificationFailed = "SentinelImageVerificationFailed"
	ReasonGatewayAPIMissing               = "GatewayAPIMissing"
	ReasonPrerequisitesMissing            = "PrerequisitesMissing"
	ReasonPrerequisitesReady              = "PrerequisitesReady"
	ReasonAdmissionPoliciesNotReady       = "AdmissionPoliciesNotReady"
	ReasonAdmissionPoliciesReady          = "AdmissionPoliciesReady"

	ReasonLeaderFound           = "LeaderFound"
	ReasonLeaderUnknown         = "LeaderUnknown"
	ReasonMultipleLeaders       = "MultipleLeaders"
	ReasonEtcdEncryptionUnknown = "EtcdEncryptionUnknown"
	ReasonDevelopmentProfile    = "DevelopmentProfile"
	ReasonProfileNotSet         = "ProfileNotSet"
	ReasonProductionReady       = "ProductionReady"
	ReasonProductionNotReady    = "ProductionNotReady"
	ReasonRootTokenStored       = "RootTokenStored"
	ReasonStaticUnsealInUse     = "StaticUnsealInUse"
	ReasonOperatorManagedTLS    = "OperatorManagedTLS"
	ReasonSecurityViolation     = "SecurityViolation"

	// ComponentSentinel is the component name for sentinel resources.
	ComponentSentinel = "sentinel"
)
