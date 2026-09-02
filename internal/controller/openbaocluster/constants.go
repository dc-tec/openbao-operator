package openbaocluster

import (
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// Reason constants for OpenBaoCluster conditions.
const (
	ReasonGatewayAPIMissing                      = constants.ReasonGatewayAPIMissing
	ReasonOIDCBootstrapConfigurationInvalid      = constants.ReasonOIDCBootstrapConfigurationInvalid
	ReasonAPIServerNetworkConfigurationInvalid   = constants.ReasonAPIServerNetworkConfigurationInvalid
	ReasonPrerequisitesMissing                   = constants.ReasonPrerequisitesMissing
	ReasonUnsafeAdmissionDisabled                = "UnsafeAdmissionDisabled"
	ReasonDevelopmentProfile                     = "DevelopmentProfile"
	ReasonAmbientUnsealIdentity                  = "AmbientUnsealIdentity"
	ReasonProfileNotSet                          = "ProfileNotSet"
	ReasonRootTokenStored                        = "RootTokenStored"
	ReasonStaticUnsealInUse                      = "StaticUnsealInUse"
	ReasonTLSSecretMissing                       = "TLSSecretMissing"
	ReasonTLSSecretInvalid                       = "TLSSecretInvalid"
	ReasonACMEIntegrationReady                   = constants.ReasonACMEIntegrationReady
	ReasonACMECacheReady                         = "ACMECacheReady"
	ReasonGatewayIntegrationReady                = constants.ReasonGatewayIntegrationReady
	ReasonIngressIntegrationReady                = constants.ReasonIngressIntegrationReady
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
	ReasonGatewayRoutePending                    = constants.ReasonGatewayRoutePending
	ReasonGatewayRouteNotAccepted                = constants.ReasonGatewayRouteNotAccepted
	ReasonGatewayRouteReferencesUnresolved       = constants.ReasonGatewayRouteReferencesUnresolved
	ReasonIngressClassMissing                    = constants.ReasonIngressClassMissing
	ReasonIngressCapabilitiesUnknown             = constants.ReasonIngressCapabilitiesUnknown
	ReasonIngressObjectPending                   = constants.ReasonIngressObjectPending
	ReasonIngressLoadBalancerPending             = constants.ReasonIngressLoadBalancerPending
	ReasonACMECacheNotConfigured                 = "ACMECacheNotConfigured"
	ReasonACMECacheMissing                       = "ACMECacheMissing"
	ReasonACMECachePending                       = "ACMECachePending"
	ReasonACMECacheInvalidAccessMode             = "ACMECacheInvalidAccessMode"
	ReasonACMEDomainNotResolvable                = constants.ReasonACMEDomainNotResolvable
	ReasonACMEGatewayNotConfiguredForPassthrough = constants.ReasonACMEGatewayNotConfiguredForPassthrough
	ReasonDisabled                               = "Disabled"

	ReasonImageVersionMismatch = constants.ReasonImageVersionMismatch

	reasonReady   = "Ready"
	reasonPaused  = "Paused"
	reasonUnknown = constants.ReasonUnknown

	controllerNameWorkload = "openbaocluster-workload"
	controllerNameAdminOps = "openbaocluster-adminops"
	controllerNameStatus   = "openbaocluster-status"

	annotationLastDevelopmentWarning     = "openbao.org/last-development-warning"
	annotationLastAmbientUnsealNote      = "openbao.org/last-ambient-unseal-identity-note"
	annotationLastProfileNotSetWarning   = "openbao.org/last-profile-not-set-warning"
	annotationLastRootTokenWarning       = "openbao.org/last-root-token-warning"
	annotationLastStaticUnsealWarning    = "openbao.org/last-static-unseal-warning"
	annotationLastUnsafeAdmissionWarning = "openbao.org/last-unsafe-admission-warning"
)

const (
	ReasonAuditFileStorageReady                       = "AuditFileStorageReady"
	ReasonAuditFileStorageMissing                     = "AuditFileStorageMissing"
	ReasonAuditFileStoragePending                     = "AuditFileStoragePending"
	ReasonAuditFileStorageInvalidAccessMode           = "AuditFileStorageInvalidAccessMode"
	ReasonAuditFileStorageStatefulSetRecreateRequired = constants.ReasonAuditFileStorageStatefulSetRecreateRequired
)

const securityWarningInterval = time.Hour
