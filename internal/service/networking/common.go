package networking

import "errors"

// ErrGatewayAPIMissing indicates that Gateway API CRDs are not installed in the
// cluster while Gateway support is enabled in the OpenBaoCluster spec. Callers
// can use this error to surface a degraded condition instead of silently
// skipping HTTPRoute reconciliation.
var ErrGatewayAPIMissing = errors.New("gateway API CRDs not installed")

// ErrGatewayReferenceMissing indicates the referenced Gateway object does not exist.
var ErrGatewayReferenceMissing = errors.New("referenced Gateway not found")

// ErrGatewayClassMissing indicates the Gateway references a GatewayClass that
// does not exist or is not specified.
var ErrGatewayClassMissing = errors.New("referenced GatewayClass not found")

// ErrGatewayClassNotAccepted indicates the GatewayClass has explicitly reported
// that it is not accepted by its controller.
var ErrGatewayClassNotAccepted = errors.New("GatewayClass not accepted")

// ErrGatewayClassPending indicates the GatewayClass has not yet reported a
// conclusive Accepted/SupportedVersion status.
var ErrGatewayClassPending = errors.New("GatewayClass status pending")

// ErrGatewayVersionUnsupported indicates the GatewayClass has explicitly
// reported that the installed Gateway API version is unsupported.
var ErrGatewayVersionUnsupported = errors.New("gateway API version unsupported")

// ErrGatewayFeatureUnsupported indicates the GatewayClass has explicitly
// reported that it does not support a feature required by the cluster config.
var ErrGatewayFeatureUnsupported = errors.New("GatewayClass feature unsupported")

// ErrGatewayCapabilitiesUnknown indicates the GatewayClass does not publish the
// supportedFeatures set required to verify the selected Gateway mode.
var ErrGatewayCapabilitiesUnknown = errors.New("GatewayClass capabilities unknown")

// ErrGatewayNotProgrammed indicates the Gateway has explicitly reported that it
// is not programmed by its controller.
var ErrGatewayNotProgrammed = errors.New("gateway not programmed")

// ErrGatewayProgrammingPending indicates the Gateway has not yet reported a
// conclusive Programmed status.
var ErrGatewayProgrammingPending = errors.New("gateway programming pending")

// ErrGatewayListenerIncompatible indicates the referenced Gateway listeners are
// incompatible with the selected HTTPRoute/TLSRoute mode.
var ErrGatewayListenerIncompatible = errors.New("gateway listener incompatible")

// ErrGatewayRoutePending indicates the operator-managed Route or its status
// has not yet converged for the relevant Gateway parent.
var ErrGatewayRoutePending = errors.New("gateway Route status pending")

// ErrGatewayRouteNotAccepted indicates the relevant Gateway parent has
// explicitly rejected the operator-managed Route.
var ErrGatewayRouteNotAccepted = errors.New("gateway Route not accepted")

// ErrGatewayRouteReferencesUnresolved indicates the relevant Gateway parent
// cannot resolve one or more references from the operator-managed Route.
var ErrGatewayRouteReferencesUnresolved = errors.New("gateway Route references unresolved")

// ErrIngressClassMissing indicates the selected IngressClass does not exist.
var ErrIngressClassMissing = errors.New("referenced IngressClass not found")

// ErrIngressCapabilitiesUnknown indicates the operator cannot verify the selected
// ingress integration because required reads are not available.
var ErrIngressCapabilitiesUnknown = errors.New("ingress capabilities unknown")

// ErrIngressObjectMissing indicates the managed Ingress object does not exist yet.
var ErrIngressObjectMissing = errors.New("managed ingress missing")

// ErrIngressLoadBalancerPending indicates the managed Ingress has not yet
// published a load balancer address in status.
var ErrIngressLoadBalancerPending = errors.New("ingress load balancer pending")

// ErrAPIServerNetworkConfigurationInvalid indicates that the operator could not
// derive a safe least-privilege Kubernetes API egress allow-list from the
// cluster spec and runtime environment.
var ErrAPIServerNetworkConfigurationInvalid = errors.New("API server network configuration invalid")
