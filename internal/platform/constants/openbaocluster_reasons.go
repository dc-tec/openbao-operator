package constants

// Shared condition reasons used across app, service, and controller layers for
// OpenBaoCluster workflows.
const (
	ReasonGatewayAPIMissing                    = "GatewayAPIMissing"
	ReasonOIDCBootstrapConfigurationInvalid    = "OIDCBootstrapConfigurationInvalid"
	ReasonAPIServerNetworkConfigurationInvalid = "APIServerNetworkConfigurationInvalid"

	ReasonACMEDomainNotResolvable                = "ACMEDomainNotResolvable"
	ReasonACMEGatewayNotConfiguredForPassthrough = "ACMEGatewayNotConfiguredForPassthrough"
	ReasonACMEIntegrationReady                   = "ACMEIntegrationReady"

	ReasonGatewayIntegrationReady         = "GatewayIntegrationReady"
	ReasonIngressIntegrationReady         = "IngressIntegrationReady"
	ReasonGatewayReferenceMissing         = "GatewayReferenceMissing"
	ReasonGatewayClassMissing             = "GatewayClassMissing"
	ReasonGatewayClassPending             = "GatewayClassPending"
	ReasonGatewayClassNotAccepted         = "GatewayClassNotAccepted"
	ReasonGatewayVersionUnsupported       = "GatewayVersionUnsupported"
	ReasonGatewayFeatureUnsupported       = "GatewayFeatureUnsupported"
	ReasonGatewayCapabilitiesUnknown      = "GatewayCapabilitiesUnknown"
	ReasonGatewayNotProgrammed            = "GatewayNotProgrammed"
	ReasonGatewayProgrammingPending       = "GatewayProgrammingPending"
	ReasonGatewayListenerIncompatible     = "GatewayListenerIncompatible"
	ReasonIngressClassMissing             = "IngressClassMissing"
	ReasonIngressCapabilitiesUnknown      = "IngressCapabilitiesUnknown"
	ReasonIngressObjectPending            = "IngressObjectPending"
	ReasonIngressLoadBalancerPending      = "IngressLoadBalancerPending"
	ReasonAPIServerNetworkReady           = "APIServerNetworkReady"
	ReasonAPIServerEndpointIPsRecommended = "APIServerEndpointIPsRecommended"

	ReasonStorageInvalidSize             = "StorageInvalidSize"
	ReasonStorageShrinkNotSupported      = "StorageShrinkNotSupported"
	ReasonStorageResizeNotSupported      = "StorageResizeNotSupported"
	ReasonStorageClassChangeNotSupported = "StorageClassChangeNotSupported"
	ReasonStorageRestartRequired         = "StorageRestartRequired"

	ReasonAuditFileStorageStatefulSetRecreateRequired = "AuditFileStorageStatefulSetRecreateRequired"
)
