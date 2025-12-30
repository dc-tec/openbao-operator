package openbaocluster

// Reason constants for OpenBaoCluster conditions.
const (
	ReasonImageVerificationFailed         = "ImageVerificationFailed"
	ReasonSentinelImageVerificationFailed = "SentinelImageVerificationFailed"
	ReasonGatewayAPIMissing               = "GatewayAPIMissing"
	ReasonPrerequisitesMissing            = "PrerequisitesMissing"
	ReasonPrerequisitesReady              = "PrerequisitesReady"

	ReasonLeaderFound           = "LeaderFound"
	ReasonLeaderUnknown         = "LeaderUnknown"
	ReasonMultipleLeaders       = "MultipleLeaders"
	ReasonEtcdEncryptionUnknown = "EtcdEncryptionUnknown"
	ReasonDevelopmentProfile    = "DevelopmentProfile"
	ReasonRootTokenStored       = "RootTokenStored"
	ReasonSecurityViolation     = "SecurityViolation"

	// ComponentSentinel is the component name for sentinel resources.
	ComponentSentinel = "sentinel"
)
