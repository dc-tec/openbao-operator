package openbaocluster

import (
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// Reason constants for OpenBaoCluster conditions.
const (
	ReasonGatewayAPIMissing                    = constants.ReasonGatewayAPIMissing
	ReasonOIDCBootstrapConfigurationInvalid    = constants.ReasonOIDCBootstrapConfigurationInvalid
	ReasonAPIServerNetworkConfigurationInvalid = constants.ReasonAPIServerNetworkConfigurationInvalid
	ReasonPrerequisitesMissing                 = constants.ReasonPrerequisitesMissing
	ReasonPrerequisitesReady                   = "PrerequisitesReady"
	ReasonAdmissionPoliciesNotReady            = "AdmissionPoliciesNotReady"
	ReasonAdmissionPoliciesReady               = "AdmissionPoliciesReady"

	ReasonInProgress = "InProgress"

	ReasonLeaderFound                            = "LeaderFound"
	ReasonLeaderUnknown                          = constants.ReasonLeaderUnknown
	ReasonMultipleLeaders                        = "MultipleLeaders"
	ReasonInitialized                            = "Initialized"
	ReasonNotInitialized                         = "NotInitialized"
	ReasonSealed                                 = "Sealed"
	ReasonUnsealed                               = "Unsealed"
	ReasonAppArmorUnsupported                    = "AppArmorUnsupported"
	ReasonEtcdEncryptionUnknown                  = "EtcdEncryptionUnknown"
	ReasonDevelopmentProfile                     = "DevelopmentProfile"
	ReasonAmbientUnsealIdentity                  = "AmbientUnsealIdentity"
	ReasonProfileNotSet                          = "ProfileNotSet"
	ReasonProductionReady                        = "ProductionReady"
	ReasonProductionNotReady                     = "ProductionNotReady"
	ReasonTransitAddressNotHTTPS                 = "TransitAddressNotHTTPS"
	ReasonTransitInlineToken                     = "TransitInlineToken"
	ReasonUnsealTLSSkipVerify                    = "UnsealTLSSkipVerify"
	ReasonSecurityContextWeakening               = "SecurityContextWeakening"
	ReasonUserAccessConfigured                   = "UserAccessConfigured"
	ReasonUserAccessUnverified                   = "UserAccessUnverified"
	ReasonRootTokenStored                        = "RootTokenStored"
	ReasonStaticUnsealInUse                      = "StaticUnsealInUse"
	ReasonOperatorManagedTLS                     = "OperatorManagedTLS"
	ReasonSecurityViolation                      = constants.ReasonSecurityViolation
	ReasonTLSSecretMissing                       = "TLSSecretMissing"
	ReasonTLSSecretInvalid                       = "TLSSecretInvalid"
	ReasonACMEIntegrationReady                   = constants.ReasonACMEIntegrationReady
	ReasonACMECacheReady                         = "ACMECacheReady"
	ReasonGatewayIntegrationReady                = constants.ReasonGatewayIntegrationReady
	ReasonAPIServerNetworkReady                  = constants.ReasonAPIServerNetworkReady
	ReasonAPIServerEndpointIPsRecommended        = constants.ReasonAPIServerEndpointIPsRecommended
	ReasonGatewayReferenceMissing                = constants.ReasonGatewayReferenceMissing
	ReasonGatewayClassMissing                    = constants.ReasonGatewayClassMissing
	ReasonGatewayClassPending                    = constants.ReasonGatewayClassPending
	ReasonGatewayClassNotAccepted                = constants.ReasonGatewayClassNotAccepted
	ReasonGatewayVersionUnsupported              = constants.ReasonGatewayVersionUnsupported
	ReasonGatewayFeatureUnsupported              = constants.ReasonGatewayFeatureUnsupported
	ReasonGatewayCapabilitiesUnknown             = constants.ReasonGatewayCapabilitiesUnknown
	ReasonGatewayNotProgrammed                   = constants.ReasonGatewayNotProgrammed
	ReasonGatewayProgrammingPending              = constants.ReasonGatewayProgrammingPending
	ReasonGatewayListenerIncompatible            = constants.ReasonGatewayListenerIncompatible
	ReasonACMECacheNotConfigured                 = "ACMECacheNotConfigured"
	ReasonACMECacheMissing                       = "ACMECacheMissing"
	ReasonACMECachePending                       = "ACMECachePending"
	ReasonACMECacheInvalidAccessMode             = "ACMECacheInvalidAccessMode"
	ReasonACMEDomainNotResolvable                = constants.ReasonACMEDomainNotResolvable
	ReasonACMEGatewayNotConfiguredForPassthrough = constants.ReasonACMEGatewayNotConfiguredForPassthrough
	ReasonDisabled                               = "Disabled"
	ReasonNotReady                               = "NotReady"
	ReasonAllReplicasReady                       = "AllReplicasReady"
	ReasonNoReplicasReady                        = "NoReplicasReady"
	ReasonNoReadReplicasConfigured               = "NoReadReplicasConfigured"
	ReasonAllReadReplicasReady                   = "AllReadReplicasReady"
	ReasonReadReplicasNotReady                   = "ReadReplicasNotReady"
	ReasonNoReadyReadReplicas                    = "NoReadyReadReplicas"
	ReasonReadServingAvailable                   = "ReadServingAvailable"
	ReasonReadServingWithoutQuorum               = "ReadServingWithoutQuorum"
	ReasonPodsNotServingReads                    = "PodsNotServingReads"
	ReasonRaftMembershipReady                    = "RaftMembershipReady"
	ReasonReadReplicasAutopilotHealthy           = "ReadReplicasAutopilotHealthy"
	ReasonReadReplicasAutopilotUnhealthy         = "ReadReplicasAutopilotUnhealthy"

	ReasonStorageInvalidSize             = constants.ReasonStorageInvalidSize
	ReasonStorageShrinkNotSupported      = constants.ReasonStorageShrinkNotSupported
	ReasonStorageResizeNotSupported      = constants.ReasonStorageResizeNotSupported
	ReasonStorageClassChangeNotSupported = constants.ReasonStorageClassChangeNotSupported
	ReasonStorageRestartRequired         = constants.ReasonStorageRestartRequired
	ReasonInvalidVersion                 = constants.ReasonInvalidVersion
	ReasonDowngradeBlocked               = constants.ReasonDowngradeBlocked
	ReasonImageVersionMismatch           = constants.ReasonImageVersionMismatch
	ReasonStorageClassConfigured         = "StorageClassConfigured"
	ReasonStorageClassPending            = "StorageClassPending"
	ReasonStorageClassDefaulted          = "StorageClassDefaulted"
	ReasonStorageClassUnset              = "StorageClassUnset"
	ReasonStorageClassMismatch           = "StorageClassMismatch"
	ReasonStorageClassInconsistent       = "StorageClassInconsistent"

	reasonReady              = "Ready"
	reasonPaused             = "Paused"
	reasonReconciling        = "Reconciling"
	reasonIdle               = "Idle"
	reasonUnknown            = constants.ReasonUnknown
	reasonBreakGlassRequired = "BreakGlassRequired"

	controllerNameWorkload = "openbaocluster-workload"
	controllerNameAdminOps = "openbaocluster-adminops"
	controllerNameStatus   = "openbaocluster-status"

	annotationLastDevelopmentWarning   = "openbao.org/last-development-warning"
	annotationLastAmbientUnsealNote    = "openbao.org/last-ambient-unseal-identity-note"
	annotationLastProfileNotSetWarning = "openbao.org/last-profile-not-set-warning"
	annotationLastRootTokenWarning     = "openbao.org/last-root-token-warning"
	annotationLastStaticUnsealWarning  = "openbao.org/last-static-unseal-warning"
)

const securityWarningInterval = time.Hour
