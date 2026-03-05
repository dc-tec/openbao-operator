package openbaocluster

import "time"

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

	reasonReady              = "Ready"
	reasonPaused             = "Paused"
	reasonReconciling        = "Reconciling"
	reasonIdle               = "Idle"
	reasonUnknown            = "Unknown"
	reasonBreakGlassRequired = "BreakGlassRequired"

	controllerNameWorkload = "openbaocluster-workload"
	controllerNameAdminOps = "openbaocluster-adminops"
	controllerNameStatus   = "openbaocluster-status"

	annotationLastDevelopmentWarning   = "openbao.org/last-development-warning"
	annotationLastProfileNotSetWarning = "openbao.org/last-profile-not-set-warning"
	annotationLastRootTokenWarning     = "openbao.org/last-root-token-warning"
	annotationLastStaticUnsealWarning  = "openbao.org/last-static-unseal-warning"
)

const securityWarningInterval = time.Hour
