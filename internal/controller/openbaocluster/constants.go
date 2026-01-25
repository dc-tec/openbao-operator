package openbaocluster

// Reason constants for OpenBaoCluster conditions.
const (
	ReasonGatewayAPIMissing         = "GatewayAPIMissing"
	ReasonPrerequisitesMissing      = "PrerequisitesMissing"
	ReasonPrerequisitesReady        = "PrerequisitesReady"
	ReasonAdmissionPoliciesNotReady = "AdmissionPoliciesNotReady"
	ReasonAdmissionPoliciesReady    = "AdmissionPoliciesReady"

	ReasonInProgress = "InProgress"

	ReasonLeaderFound                            = "LeaderFound"
	ReasonLeaderUnknown                          = "LeaderUnknown"
	ReasonMultipleLeaders                        = "MultipleLeaders"
	ReasonEtcdEncryptionUnknown                  = "EtcdEncryptionUnknown"
	ReasonDevelopmentProfile                     = "DevelopmentProfile"
	ReasonProfileNotSet                          = "ProfileNotSet"
	ReasonProductionReady                        = "ProductionReady"
	ReasonProductionNotReady                     = "ProductionNotReady"
	ReasonRootTokenStored                        = "RootTokenStored"
	ReasonStaticUnsealInUse                      = "StaticUnsealInUse"
	ReasonOperatorManagedTLS                     = "OperatorManagedTLS"
	ReasonSecurityViolation                      = "SecurityViolation"
	ReasonTLSSecretMissing                       = "TLSSecretMissing"
	ReasonTLSSecretInvalid                       = "TLSSecretInvalid"
	ReasonACMEDomainNotResolvable                = "ACMEDomainNotResolvable"
	ReasonACMEGatewayNotConfiguredForPassthrough = "ACMEGatewayNotConfiguredForPassthrough"
	ReasonDisabled                               = "Disabled"
	ReasonNotReady                               = "NotReady"
	ReasonAllReplicasReady                       = "AllReplicasReady"
	ReasonNoReplicasReady                        = "NoReplicasReady"

	ReasonStorageInvalidSize             = "StorageInvalidSize"
	ReasonStorageShrinkNotSupported      = "StorageShrinkNotSupported"
	ReasonStorageResizeNotSupported      = "StorageResizeNotSupported"
	ReasonStorageClassChangeNotSupported = "StorageClassChangeNotSupported"
	ReasonStorageRestartRequired         = "StorageRestartRequired"
)
