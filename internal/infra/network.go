package infra

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

// ErrGatewayAPIMissing indicates that Gateway API CRDs are not installed in the
// cluster while Gateway support is enabled in the OpenBaoCluster spec. Callers
// can use this error to surface a degraded condition instead of silently
// skipping HTTPRoute reconciliation.
var ErrGatewayAPIMissing = errors.New("gateway API CRDs not installed")

// ensureHeadlessService manages the headless service for stable network IDs.
// ensureHeadlessService manages the headless Service for the OpenBaoCluster using Server-Side Apply.
func (m *Manager) ensureHeadlessService(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	svcName := headlessServiceName(cluster)

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector:                 podSelectorLabels(cluster),
			Ports: []corev1.ServicePort{
				{
					Name:     "api",
					Port:     constants.PortAPI,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := m.applyResource(ctx, service, cluster); err != nil {
		return fmt.Errorf("failed to ensure headless Service %s/%s: %w", cluster.Namespace, svcName, err)
	}

	return nil
}

// ensureExternalService manages the external-facing Service for the OpenBaoCluster using Server-Side Apply.
func (m *Manager) ensureExternalService(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	serviceCfg := cluster.Spec.Service
	ingressCfg := cluster.Spec.Ingress
	gatewayCfg := cluster.Spec.Gateway

	needsService := serviceCfg != nil ||
		(ingressCfg != nil && ingressCfg.Enabled) ||
		(gatewayCfg != nil && gatewayCfg.Enabled)
	svcName := externalServiceName(cluster)

	// If service is not needed, check if it exists and delete it
	if !needsService {
		// Delete main external service
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, svcName); err != nil {
			return fmt.Errorf("failed to delete external Service %s/%s: %w", cluster.Namespace, svcName, err)
		}
		// Delete any blue/green-specific services that might exist from previous runs
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameBlue(cluster)); err != nil {
			return fmt.Errorf("failed to delete blue external Service %s/%s: %w", cluster.Namespace, externalServiceNameBlue(cluster), err)
		}
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameGreen(cluster)); err != nil {
			return fmt.Errorf("failed to delete green external Service %s/%s: %w", cluster.Namespace, externalServiceNameGreen(cluster), err)
		}
		return nil
	}

	// Build the desired service spec
	svcType := corev1.ServiceTypeClusterIP
	annotations := map[string]string{}
	if serviceCfg != nil {
		if serviceCfg.Type != "" {
			svcType = serviceCfg.Type
		}
		for k, v := range serviceCfg.Annotations {
			annotations[k] = v
		}
	}

	selectorLabels := podSelectorLabels(cluster)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			// During blue/green upgrades, select the active revision (Blue by default,
			// Green after cutover, which happens at DemotingBlue/Cleanup).
			activeRevision := cluster.Status.BlueGreen.BlueRevision
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				if cluster.Status.BlueGreen.GreenRevision != "" {
					activeRevision = cluster.Status.BlueGreen.GreenRevision
				}
			}
			selectorLabels[constants.LabelOpenBaoRevision] = activeRevision
		}
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcName,
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:     "api",
					Port:     constants.PortAPI,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := m.applyResource(ctx, service, cluster); err != nil {
		return fmt.Errorf("failed to ensure external Service %s/%s: %w", cluster.Namespace, svcName, err)
	}

	// Gateway-weighted traffic switching was removed. Clean up any stale Services
	// from previous iterations that used revision-specific HTTPRoute backends.
	if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameBlue(cluster)); err != nil {
		return fmt.Errorf("failed to delete stale blue external Service: %w", err)
	}
	if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameGreen(cluster)); err != nil {
		return fmt.Errorf("failed to delete stale green external Service: %w", err)
	}

	return nil
}

// ensureACMEChallengeService manages a dedicated Service for ACME validation in ACME TLS mode.
//
// In ACME mode, OpenBao must complete ACME challenges before it can become Ready (it has no
// serving certificate yet). Most Kubernetes Services only publish ready pod endpoints, creating
// a circular dependency. This Service sets PublishNotReadyAddresses so ACME validators can reach
// pods while they are still initializing.
//
// The Service exposes standard ACME ports (80/443) and forwards to the OpenBao listener port
// (8200). This is particularly useful for private ACME CAs running inside the cluster.
func (m *Manager) ensureACMEChallengeService(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	enabled := usesACMEMode(cluster)
	svcName := acmeServiceName(cluster)

	if !enabled {
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, svcName); err != nil {
			return fmt.Errorf("failed to delete ACME challenge Service %s/%s: %w", cluster.Namespace, svcName, err)
		}
		return nil
	}

	selectorLabels := podSelectorLabels(cluster)
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			activeRevision := cluster.Status.BlueGreen.BlueRevision
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				if cluster.Status.BlueGreen.GreenRevision != "" {
					activeRevision = cluster.Status.BlueGreen.GreenRevision
				}
			}
			selectorLabels[constants.LabelOpenBaoRevision] = activeRevision
		}
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			PublishNotReadyAddresses: true,
			Selector:                 selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http-80",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(constants.PortAPI),
				},
				{
					Name:       "https-443",
					Protocol:   corev1.ProtocolTCP,
					Port:       443,
					TargetPort: intstr.FromInt(constants.PortAPI),
				},
			},
		},
	}

	if err := m.applyResource(ctx, service, cluster); err != nil {
		return fmt.Errorf("failed to ensure ACME challenge Service %s/%s: %w", cluster.Namespace, svcName, err)
	}

	return nil
}

// ensureIngress manages external access via Ingress using Server-Side Apply.
func (m *Manager) ensureIngress(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	ingressCfg := cluster.Spec.Ingress
	enabled := ingressCfg != nil && ingressCfg.Enabled
	name := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}

	return reconcileOptionalResource(ctx, optionalResourceOptions{
		kind:              "Ingress",
		apiVersion:        "networking.k8s.io/v1",
		enabled:           enabled,
		name:              name,
		logger:            logger,
		logKey:            "ingress",
		deleteDisabledMsg: "Ingress no longer enabled; deleting",
		deleteInvalidMsg:  "Ingress configuration invalid; deleting existing Ingress",
		newEmpty: func() client.Object {
			return &networkingv1.Ingress{}
		},
		buildDesired: func() (client.Object, bool, error) {
			desired := buildIngress(cluster)
			if desired == nil {
				return nil, false, nil
			}
			return desired, true, nil
		},
		get:    m.client.Get,
		delete: func(ctx context.Context, obj client.Object) error { return m.client.Delete(ctx, obj) },
		apply:  func(ctx context.Context, obj client.Object) error { return m.applyResource(ctx, obj, cluster) },
	})
}

// buildIngress constructs an Ingress resource for the given OpenBaoCluster.
func buildIngress(cluster *openbaov1alpha1.OpenBaoCluster) *networkingv1.Ingress {
	if cluster.Spec.Ingress == nil || !cluster.Spec.Ingress.Enabled {
		return nil
	}

	ing := cluster.Spec.Ingress
	if strings.TrimSpace(ing.Host) == "" {
		return nil
	}

	path := ing.Path
	if strings.TrimSpace(path) == "" {
		path = "/"
	}

	pathType := networkingv1.PathTypePrefix

	backendServiceName := externalServiceName(cluster)

	rule := networkingv1.IngressRule{
		Host: ing.Host,
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path:     path,
						PathType: &pathType,
						Backend: networkingv1.IngressBackend{
							Service: &networkingv1.IngressServiceBackend{
								Name: backendServiceName,
								Port: networkingv1.ServiceBackendPort{
									Number: constants.PortAPI,
								},
							},
						},
					},
				},
			},
		},
	}

	var tls []networkingv1.IngressTLS
	secretName := ing.TLSSecretName
	if strings.TrimSpace(secretName) == "" {
		secretName = tlsServerSecretName(cluster)
	}
	tls = append(tls, networkingv1.IngressTLS{
		Hosts:      []string{ing.Host},
		SecretName: secretName,
	})

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cluster.Name,
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: ing.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{rule},
			TLS:   tls,
		},
	}

	if ing.ClassName != nil && strings.TrimSpace(*ing.ClassName) != "" {
		className := strings.TrimSpace(*ing.ClassName)
		ingress.Spec.IngressClassName = &className
	}

	return ingress
}

// ensureHTTPRoute manages the Gateway API HTTPRoute for the OpenBaoCluster.
// When spec.gateway.enabled is true and spec.gateway.tlsPassthrough is false,
// it creates or updates an HTTPRoute that routes traffic from the referenced Gateway
// to the OpenBao public Service.
//
// This function gracefully handles the case where Gateway API CRDs are not installed
// in the cluster. If the HTTPRoute CRD is not found, the function logs a warning
// and returns nil to allow other reconciliation to continue.
func (m *Manager) ensureHTTPRoute(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	gatewayCfg := cluster.Spec.Gateway
	enabled := gatewayCfg != nil && gatewayCfg.Enabled && !gatewayCfg.TLSPassthrough
	name := types.NamespacedName{Namespace: cluster.Namespace, Name: httpRouteName(cluster)}

	return reconcileOptionalResource(ctx, optionalResourceOptions{
		kind:              "HTTPRoute",
		apiVersion:        "gateway.networking.k8s.io/v1",
		enabled:           enabled,
		name:              name,
		logger:            logger,
		logKey:            "httproute",
		deleteDisabledMsg: "HTTPRoute no longer enabled; deleting",
		deleteInvalidMsg:  "HTTPRoute configuration invalid; deleting existing HTTPRoute",
		newEmpty: func() client.Object {
			return &gatewayv1.HTTPRoute{}
		},
		buildDesired: func() (client.Object, bool, error) {
			desired := buildHTTPRoute(cluster)
			if desired == nil {
				return nil, false, nil
			}
			return desired, true, nil
		},
		degradeOnCRDMissing: true,
		get:                 m.client.Get,
		delete:              func(ctx context.Context, obj client.Object) error { return m.client.Delete(ctx, obj) },
		apply:               func(ctx context.Context, obj client.Object) error { return m.applyResource(ctx, obj, cluster) },
	})
}

// buildHTTPRoute constructs an HTTPRoute for the given OpenBaoCluster.
// Returns nil if the Gateway configuration is invalid or incomplete, or if TLS passthrough is enabled.
func buildHTTPRoute(cluster *openbaov1alpha1.OpenBaoCluster) *gatewayv1.HTTPRoute {
	if cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		return nil
	}

	// Skip HTTPRoute if TLS passthrough is enabled (TLSRoute will be used instead)
	if cluster.Spec.Gateway.TLSPassthrough {
		return nil
	}

	gw := cluster.Spec.Gateway
	if strings.TrimSpace(gw.Hostname) == "" {
		return nil
	}

	if strings.TrimSpace(gw.GatewayRef.Name) == "" {
		return nil
	}

	path := gw.Path
	if strings.TrimSpace(path) == "" {
		path = "/"
	}

	// Determine the Gateway namespace; defaults to the OpenBaoCluster namespace
	gatewayNamespace := gw.GatewayRef.Namespace
	if strings.TrimSpace(gatewayNamespace) == "" {
		gatewayNamespace = cluster.Namespace
	}

	hostname := gatewayv1.Hostname(gw.Hostname)
	pathType := gatewayv1.PathMatchPathPrefix
	port := gatewayv1.PortNumber(constants.PortAPI)
	gatewayNS := gatewayv1.Namespace(gatewayNamespace)
	var sectionName *gatewayv1.SectionName
	if strings.TrimSpace(gw.ListenerName) != "" {
		sn := gatewayv1.SectionName(strings.TrimSpace(gw.ListenerName))
		sectionName = &sn
	}

	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        httpRouteName(cluster),
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: gw.Annotations,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName(gw.GatewayRef.Name),
						Namespace:   &gatewayNS,
						SectionName: sectionName,
					},
				},
			},
			Hostnames: []gatewayv1.Hostname{hostname},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  &pathType,
								Value: &path,
							},
						},
					},
					BackendRefs: buildHTTPRouteBackends(cluster, port),
				},
			},
		},
	}

	return httpRoute
}

func buildHTTPRouteBackends(cluster *openbaov1alpha1.OpenBaoCluster, port gatewayv1.PortNumber) []gatewayv1.HTTPBackendRef {
	name := gatewayv1.ObjectName(externalServiceName(cluster))
	return []gatewayv1.HTTPBackendRef{
		{
			BackendRef: gatewayv1.BackendRef{
				BackendObjectReference: gatewayv1.BackendObjectReference{
					Name: name,
					Port: &port,
				},
			},
		},
	}
}

// httpRouteName returns the name for the HTTPRoute resource.
func httpRouteName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + httpRouteSuffix
}

// ensureTLSRoute manages the Gateway API TLSRoute for the OpenBaoCluster using Server-Side Apply.
// When spec.gateway.enabled is true and spec.gateway.tlsPassthrough is true,
// it creates or updates a TLSRoute that routes encrypted TLS traffic based on SNI
// from the referenced Gateway to the OpenBao public Service without terminating TLS.
//
// This function gracefully handles the case where Gateway API CRDs are not installed
// in the cluster. If the TLSRoute CRD is not found, it returns ErrGatewayAPIMissing
// so the caller can surface a degraded condition.
func (m *Manager) ensureTLSRoute(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	gatewayCfg := cluster.Spec.Gateway
	enabled := gatewayCfg != nil && gatewayCfg.Enabled && gatewayCfg.TLSPassthrough
	name := types.NamespacedName{Namespace: cluster.Namespace, Name: tlsRouteName(cluster)}

	return reconcileOptionalResource(ctx, optionalResourceOptions{
		kind:              "TLSRoute",
		apiVersion:        "gateway.networking.k8s.io/v1alpha2",
		enabled:           enabled,
		name:              name,
		logger:            logger,
		logKey:            "tlsroute",
		deleteDisabledMsg: "TLSRoute no longer enabled; deleting",
		deleteInvalidMsg:  "TLSRoute configuration invalid; deleting existing TLSRoute",
		newEmpty: func() client.Object {
			return &gatewayv1alpha2.TLSRoute{}
		},
		buildDesired: func() (client.Object, bool, error) {
			desired := buildTLSRoute(cluster)
			if desired == nil {
				return nil, false, nil
			}
			return desired, true, nil
		},
		degradeOnCRDMissing: true,
		get:                 m.client.Get,
		delete:              func(ctx context.Context, obj client.Object) error { return m.client.Delete(ctx, obj) },
		apply:               func(ctx context.Context, obj client.Object) error { return m.applyResource(ctx, obj, cluster) },
	})
}

// buildTLSRoute constructs a TLSRoute for the given OpenBaoCluster.
// Returns nil if the Gateway configuration is invalid or incomplete, or if TLS passthrough is disabled.
func buildTLSRoute(cluster *openbaov1alpha1.OpenBaoCluster) *gatewayv1alpha2.TLSRoute {
	if cluster.Spec.Gateway == nil || !cluster.Spec.Gateway.Enabled {
		return nil
	}

	// Only create TLSRoute if TLS passthrough is enabled
	if !cluster.Spec.Gateway.TLSPassthrough {
		return nil
	}

	gw := cluster.Spec.Gateway
	if strings.TrimSpace(gw.Hostname) == "" {
		return nil
	}

	if strings.TrimSpace(gw.GatewayRef.Name) == "" {
		return nil
	}

	// Determine the Gateway namespace; defaults to the OpenBaoCluster namespace
	gatewayNamespace := gw.GatewayRef.Namespace
	if strings.TrimSpace(gatewayNamespace) == "" {
		gatewayNamespace = cluster.Namespace
	}

	backendServiceName := externalServiceName(cluster)
	hostname := gatewayv1alpha2.Hostname(gw.Hostname)
	port := gatewayv1alpha2.PortNumber(constants.PortAPI)
	gatewayNS := gatewayv1alpha2.Namespace(gatewayNamespace)
	var sectionName *gatewayv1alpha2.SectionName
	if strings.TrimSpace(gw.ListenerName) != "" {
		sn := gatewayv1alpha2.SectionName(strings.TrimSpace(gw.ListenerName))
		sectionName = &sn
	}

	tlsRoute := &gatewayv1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:        tlsRouteName(cluster),
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: gw.Annotations,
		},
		Spec: gatewayv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayv1alpha2.ParentReference{
					{
						Name:        gatewayv1alpha2.ObjectName(gw.GatewayRef.Name),
						Namespace:   &gatewayNS,
						SectionName: sectionName,
					},
				},
			},
			Hostnames: []gatewayv1alpha2.Hostname{hostname},
			Rules: []gatewayv1alpha2.TLSRouteRule{
				{
					BackendRefs: []gatewayv1alpha2.BackendRef{
						{
							BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
								Name: gatewayv1alpha2.ObjectName(backendServiceName),
								Port: &port,
							},
						},
					},
				},
			},
		},
	}

	return tlsRoute
}

// tlsRouteName returns the name for the TLSRoute resource.
func tlsRouteName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + tlsRouteSuffix
}

// ensureBackendTLSPolicy manages the Gateway API BackendTLSPolicy for the OpenBaoCluster.
// When spec.gateway.enabled is true and spec.gateway.backendTLS.enabled is true (default),
// it creates or updates a BackendTLSPolicy that configures the Gateway to use HTTPS when
// communicating with the OpenBao backend service and validates the backend certificate
// using the cluster's CA certificate.
//
// BackendTLSPolicy is not needed when TLS passthrough is enabled (TLSRoute) since the Gateway
// does not decrypt traffic and therefore does not need to validate backend certificates.
//
// This function gracefully handles the case where Gateway API CRDs are not installed
// in the cluster. If the BackendTLSPolicy CRD is not found, the function logs a warning
// and returns nil to allow other reconciliation to continue.
func (m *Manager) ensureBackendTLSPolicy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	gatewayCfg := cluster.Spec.Gateway
	gatewayEnabled := gatewayCfg != nil && gatewayCfg.Enabled

	// BackendTLSPolicy is not needed when TLS passthrough is enabled
	if gatewayCfg != nil && gatewayCfg.TLSPassthrough {
		name := backendTLSPolicyName(cluster)
		backendTLSPolicy := &gatewayv1.BackendTLSPolicy{}
		err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      name,
		}, backendTLSPolicy)
		if err != nil {
			if operatorerrors.IsCRDMissingError(err) {
				return nil // CRD not installed, nothing to do
			}
			if apierrors.IsNotFound(err) {
				return nil // Already deleted, nothing to do
			}
			return fmt.Errorf("failed to get BackendTLSPolicy %s/%s: %w", cluster.Namespace, name, err)
		}

		logger.V(1).Info("BackendTLSPolicy not needed with TLS passthrough; deleting", "backendtlspolicy", name)
		if err := m.client.Delete(ctx, backendTLSPolicy); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete BackendTLSPolicy %s/%s: %w", cluster.Namespace, name, err)
		}
		return nil
	}

	// BackendTLS is enabled by default when Gateway is enabled
	backendTLSEnabled := gatewayEnabled
	if gatewayCfg != nil && gatewayCfg.BackendTLS != nil && gatewayCfg.BackendTLS.Enabled != nil {
		backendTLSEnabled = *gatewayCfg.BackendTLS.Enabled
	}

	// BackendTLSPolicy requires TLS to be enabled
	if backendTLSEnabled && !cluster.Spec.TLS.Enabled {
		logger.V(1).Info("BackendTLSPolicy requires TLS to be enabled; skipping", "tls_enabled", cluster.Spec.TLS.Enabled)
		return nil
	}

	name := types.NamespacedName{Namespace: cluster.Namespace, Name: backendTLSPolicyName(cluster)}

	return reconcileOptionalResource(ctx, optionalResourceOptions{
		kind:              "BackendTLSPolicy",
		apiVersion:        "gateway.networking.k8s.io/v1",
		enabled:           backendTLSEnabled,
		name:              name,
		logger:            logger,
		logKey:            "backendtlspolicy",
		deleteDisabledMsg: "BackendTLSPolicy no longer enabled; deleting",
		deleteInvalidMsg:  "BackendTLSPolicy configuration invalid; deleting existing BackendTLSPolicy",
		newEmpty: func() client.Object {
			return &gatewayv1.BackendTLSPolicy{}
		},
		buildDesired: func() (client.Object, bool, error) {
			desired := buildBackendTLSPolicy(cluster)
			if desired == nil {
				return nil, false, nil
			}
			return desired, true, nil
		},
		degradeOnCRDMissing: true,
		get:                 m.client.Get,
		delete:              func(ctx context.Context, obj client.Object) error { return m.client.Delete(ctx, obj) },
		apply:               func(ctx context.Context, obj client.Object) error { return m.applyResource(ctx, obj, cluster) },
	})
}

// buildBackendTLSPolicy constructs a BackendTLSPolicy for the given OpenBaoCluster.
// Returns nil if the Gateway configuration is invalid, incomplete, or TLS is not enabled.
func buildBackendTLSPolicy(cluster *openbaov1alpha1.OpenBaoCluster) *gatewayv1.BackendTLSPolicy {
	gatewayCfg := cluster.Spec.Gateway
	if gatewayCfg == nil || !gatewayCfg.Enabled {
		return nil
	}

	// BackendTLSPolicy requires TLS to be enabled
	if !cluster.Spec.TLS.Enabled {
		return nil
	}

	// BackendTLS is enabled by default when Gateway is enabled
	backendTLSEnabled := true
	if gatewayCfg.BackendTLS != nil && gatewayCfg.BackendTLS.Enabled != nil {
		backendTLSEnabled = *gatewayCfg.BackendTLS.Enabled
	}

	if !backendTLSEnabled {
		return nil
	}

	backendServiceName := externalServiceName(cluster)
	caConfigMapName := cluster.Name + constants.SuffixTLSCA

	// Determine hostname - use custom hostname if specified, otherwise derive from Service DNS name
	hostname := ""
	if gatewayCfg.BackendTLS != nil {
		hostname = gatewayCfg.BackendTLS.Hostname
	}
	if strings.TrimSpace(hostname) == "" {
		// Default to Service DNS name: <service-name>.<namespace>.svc
		hostname = fmt.Sprintf("%s.%s.svc", backendServiceName, cluster.Namespace)
	}

	// Build target refs - always include the main public service
	targetRefs := []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
		{
			LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
				Group: gatewayv1.Group(""),
				Kind:  gatewayv1.Kind("Service"),
				Name:  gatewayv1.ObjectName(backendServiceName),
			},
		},
	}

	backendTLSPolicy := &gatewayv1.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendTLSPolicyName(cluster),
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: targetRefs,
			Validation: gatewayv1.BackendTLSPolicyValidation{
				CACertificateRefs: []gatewayv1.LocalObjectReference{
					{
						Group: "",
						Kind:  "ConfigMap",
						Name:  gatewayv1.ObjectName(caConfigMapName),
					},
				},
				Hostname: gatewayv1.PreciseHostname(hostname),
			},
		},
	}

	return backendTLSPolicy
}

// backendTLSPolicyName returns the name for the BackendTLSPolicy resource.
func backendTLSPolicyName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + backendTLSPolicySuffix
}

// ensureNetworkPolicy creates or updates a NetworkPolicy to enforce cluster isolation.
// The NetworkPolicy implements a default-deny-all-ingress policy, only allowing:
// - Traffic from pods within the same cluster (via pod selector labels)
// - Traffic from OpenBao operator pods on port 8200 (for health checks, initialization, upgrades)
// - DNS traffic (for service discovery)
//
// Note: NetworkPolicies operate at L3/L4 (network layer) and can only restrict by
// source, destination, port, and protocol. They cannot restrict specific HTTP paths.
// Endpoint-level protection is provided by OpenBao's authentication and authorization.
//
// The operator connects to OpenBao pods on port 8200 for:
// - GET /v1/sys/health (init manager, upgrade manager)
// - PUT /v1/sys/init (init manager, standard clusters only)
// - PUT /v1/sys/step-down (upgrade manager)
//
// This enforces the network isolation described in the threat model and prevents
// unauthorized pods from accessing OpenBao cluster pods.
// ensureNetworkPolicy creates or updates a NetworkPolicy to enforce cluster isolation using Server-Side Apply.
func (m *Manager) ensureNetworkPolicy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	name := networkPolicyName(cluster)

	// Detect API server information for NetworkPolicy rules
	// SECURITY: We require API server detection to succeed to enforce least privilege.
	// Falling back to permissive namespace selectors violates Zero Trust principles.
	apiServerInfo, err := m.detectAPIServerInfo(ctx, logger, cluster)
	if err != nil {
		return fmt.Errorf("failed to detect API server information for NetworkPolicy: %w. "+
			"API server detection is required to enforce least-privilege egress rules. "+
			"Consider explicitly configuring spec.network.apiServerCIDR if auto-detection fails", err)
	}
	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return fmt.Errorf("API server information is incomplete (no service CIDR or endpoint IPs detected). " +
			"This is required to enforce least-privilege NetworkPolicy egress rules. " +
			"Consider explicitly configuring spec.network.apiServerCIDR")
	}

	desired, err := buildNetworkPolicy(cluster, apiServerInfo, m.operatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to build NetworkPolicy: %w", err)
	}

	// Set TypeMeta for SSA
	desired.TypeMeta = metav1.TypeMeta{
		Kind:       "NetworkPolicy",
		APIVersion: "networking.k8s.io/v1",
	}

	if err := m.applyResource(ctx, desired, cluster); err != nil {
		return fmt.Errorf("failed to ensure NetworkPolicy %s/%s: %w", cluster.Namespace, name, err)
	}

	return nil
}

// ensureJobNetworkPolicy creates or updates a NetworkPolicy that applies to
// backup/restore/upgrade-snapshot Jobs. These pods are excluded from the main
// OpenBao pod NetworkPolicy because they often need different egress (e.g. object
// storage), but they should still run under explicit network constraints.
func (m *Manager) ensureJobNetworkPolicy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	name := jobNetworkPolicyName(cluster)

	apiServerInfo, err := m.detectAPIServerInfo(ctx, logger, cluster)
	if err != nil {
		return fmt.Errorf("failed to detect API server information for Job NetworkPolicy: %w. "+
			"API server detection is required to enforce least-privilege egress rules. "+
			"Consider explicitly configuring spec.network.apiServerCIDR if auto-detection fails", err)
	}
	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return fmt.Errorf("API server information is incomplete (no service CIDR or endpoint IPs detected). " +
			"This is required to enforce least-privilege NetworkPolicy egress rules. " +
			"Consider explicitly configuring spec.network.apiServerCIDR")
	}

	desired, err := buildJobNetworkPolicy(cluster, apiServerInfo)
	if err != nil {
		return fmt.Errorf("failed to build Job NetworkPolicy: %w", err)
	}

	desired.TypeMeta = metav1.TypeMeta{
		Kind:       "NetworkPolicy",
		APIVersion: "networking.k8s.io/v1",
	}

	if err := m.applyResource(ctx, desired, cluster); err != nil {
		return fmt.Errorf("failed to ensure Job NetworkPolicy %s/%s: %w", cluster.Namespace, name, err)
	}

	return nil
}

// apiServerInfo contains detected information about the Kubernetes API server
// for use in NetworkPolicy IPBlock rules.
type apiServerInfo struct {
	// ServiceNetworkCIDR is a single-host CIDR that represents the `kubernetes`
	// Service ClusterIP (e.g., "10.43.0.1/32" or "fd00::1/128").
	// This allows least-privilege access to the in-cluster API service VIP on port 443.
	ServiceNetworkCIDR string
	// EndpointIPs are optional explicit API server endpoint IPs (e.g., control plane node IPs)
	// to allow direct access to the API server on port 6443 in locked-down environments.
	EndpointIPs []string
}

func ipToSingleHostCIDR(ip string) (string, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", fmt.Errorf("invalid IP address %q", ip)
	}
	if parsed.To4() != nil {
		return parsed.String() + "/32", nil
	}
	return parsed.String() + "/128", nil
}

func kubernetesServiceIPCIDRFromEnv() (string, bool) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	if host == "" {
		return "", false
	}
	cidr, err := ipToSingleHostCIDR(host)
	if err != nil {
		return "", false
	}
	return cidr, true
}

// detectAPIServerInfo detects the Kubernetes API server information needed for NetworkPolicy rules.
//
// Primary detection uses the in-cluster service VIP injected into the pod environment
// (KUBERNETES_SERVICE_HOST) so it works under namespace-scoped RBAC (single-tenant mode).
//
// API server endpoint IPs are not auto-detected; they can be configured explicitly via
// spec.network.apiServerEndpointIPs if needed.
func (m *Manager) detectAPIServerInfo(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*apiServerInfo, error) {
	info := &apiServerInfo{}
	reader := m.reader
	if reader == nil {
		reader = m.client
	}
	discovery := newAPIServerDiscovery(reader)

	manualCIDRConfigured := false
	if cluster.Spec.Network != nil && strings.TrimSpace(cluster.Spec.Network.APIServerCIDR) != "" {
		rawCIDR := strings.TrimSpace(cluster.Spec.Network.APIServerCIDR)
		_, ipNet, err := net.ParseCIDR(rawCIDR)
		if err != nil {
			return nil, fmt.Errorf("invalid spec.network.apiServerCIDR %q: %w", rawCIDR, err)
		}
		ipNet.IP = ipNet.IP.Mask(ipNet.Mask)
		canonicalCIDR := ipNet.String()
		logger.V(1).Info("Using manually configured API server CIDR", "cidr", canonicalCIDR)
		if canonicalCIDR != rawCIDR {
			logger.V(1).Info("Normalized API server CIDR", "original", rawCIDR, "normalized", canonicalCIDR)
		}
		info.ServiceNetworkCIDR = canonicalCIDR
		manualCIDRConfigured = true
	}

	if cluster.Spec.Network != nil && len(cluster.Spec.Network.APIServerEndpointIPs) > 0 {
		for _, rawIP := range cluster.Spec.Network.APIServerEndpointIPs {
			ip := strings.TrimSpace(rawIP)
			if ip == "" {
				continue
			}
			parsed := net.ParseIP(ip)
			if parsed == nil {
				return nil, fmt.Errorf("invalid spec.network.apiServerEndpointIPs entry %q: must be an IP address", rawIP)
			}
			info.EndpointIPs = append(info.EndpointIPs, parsed.String())
		}

		if len(info.EndpointIPs) > 0 {
			logger.V(1).Info("Using manually configured API server endpoint IPs", "ips", info.EndpointIPs)
		}
	}

	// Primary, RBAC-free detection path: use the service IP injected into the Pod environment.
	// This is the same source used by in-cluster client-go config and works in single-tenant
	// namespace-scoped installs without cross-namespace reads.
	if !manualCIDRConfigured {
		if cidr, ok := kubernetesServiceIPCIDRFromEnv(); ok {
			info.ServiceNetworkCIDR = cidr
			logger.V(1).Info("Using kubernetes Service IP CIDR from environment", "cidr", cidr)
		}
	}

	// Fallback only if env vars are missing/unparseable and no manual CIDR is configured.
	if !manualCIDRConfigured && info.ServiceNetworkCIDR == "" {
		serviceNetworkCIDR, err := discovery.DiscoverServiceNetworkCIDR(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to detect kubernetes Service IP CIDR (env KUBERNETES_SERVICE_HOST missing/unusable): %w. "+
				"Consider configuring spec.network.apiServerCIDR as a fallback", err)
		}
		if serviceNetworkCIDR != "" {
			info.ServiceNetworkCIDR = serviceNetworkCIDR
			logger.V(1).Info("Detected kubernetes service IP CIDR", "cidr", info.ServiceNetworkCIDR)
		}
	}

	// Note: We intentionally do not auto-detect API server endpoint IPs.
	// Some CNI/NetworkPolicy implementations enforce egress rules on post-DNAT traffic, so
	// allowing only the kubernetes service VIP (port 443) may not be sufficient if traffic is
	// evaluated against the backing endpoint IP/port (commonly port 6443).
	//
	// In those environments, users must configure spec.network.apiServerEndpointIPs to add
	// explicit /32 or /128 egress allow rules for the control plane endpoint(s) on port 6443.

	return info, nil
}

// buildNetworkPolicyIngressRules constructs the ingress rules for the NetworkPolicy.
// It dynamically includes rules for Gateway controllers based on the cluster configuration.
func buildNetworkPolicyIngressRules(
	cluster *openbaov1alpha1.OpenBaoCluster,
	clusterPeer, operatorPeer networkingv1.NetworkPolicyPeer,
	apiPort, clusterPort intstr.IntOrString,
) []networkingv1.NetworkPolicyIngressRule {
	rules := []networkingv1.NetworkPolicyIngressRule{
		{
			// Allow ingress from pods within the same cluster
			From: []networkingv1.NetworkPolicyPeer{clusterPeer},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &apiPort,
				},
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &clusterPort,
				},
			},
		},
	}

	// If Gateway is enabled, allow ingress from the Gateway namespace
	// This enables external traffic routing through Gateway/HTTPRoute or TLSRoute
	// (for TLS passthrough). NetworkPolicies operate at L3/L4, so both HTTPRoute
	// (with TLS termination) and TLSRoute (with TLS passthrough) work the same
	// way from a network policy perspective - both are TCP traffic on port 8200.
	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		gatewayNamespace := cluster.Spec.Gateway.GatewayRef.Namespace
		if strings.TrimSpace(gatewayNamespace) == "" {
			// Default to cluster namespace if not specified
			gatewayNamespace = cluster.Namespace
		}

		// Only add rule if Gateway is in a different namespace
		// (if same namespace, clusterPeer already covers it)
		if gatewayNamespace != cluster.Namespace {
			gatewayPeer := networkingv1.NetworkPolicyPeer{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": gatewayNamespace,
					},
				},
			}
			rules = append(rules, networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{gatewayPeer},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &apiPort,
					},
				},
			})
		}
	}

	// Always allow ingress from OpenBao operator pods on port 8200
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		// Allow ingress from OpenBao operator pods on port 8200.
		// Used for: GET /v1/sys/health, PUT /v1/sys/init, PUT /v1/sys/step-down
		From: []networkingv1.NetworkPolicyPeer{operatorPeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
		},
	})

	// Allow ingress from backup and restore pods on port 8200.
	// These pods are labeled with openbao.org/cluster=<cluster-name> and
	// openbao.org/component in (backup, restore). They need to access the
	// leader to perform snapshot/restore operations.
	backupRestorePeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.LabelOpenBaoCluster: cluster.Name,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "openbao.org/component",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"backup", "restore"},
				},
			},
		},
	}
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{backupRestorePeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
		},
	})

	// If standard Ingress is enabled, we must allow traffic to the API port.
	// Since Ingress Controllers can run anywhere (and often preserve client IPs),
	// we allow traffic from anywhere on the API port.
	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &apiPort,
				},
			},
			// Empty "From" implies "Allow from anywhere"
			From: []networkingv1.NetworkPolicyPeer{},
		})
	}

	return rules
}

// buildNetworkPolicy constructs a NetworkPolicy for the given OpenBaoCluster.
// The policy enforces:
// - Default deny all ingress traffic
// - Allow ingress from pods within the same cluster (same pod selector labels)
// - Allow ingress from Gateway namespace (if Gateway is enabled and in different namespace)
// - Allow ingress from OpenBao operator pods on port 8200 (for health checks, initialization, upgrades)
// - Allow egress to DNS (port 53 UDP/TCP) for service discovery
// - Allow egress to Kubernetes API server via service network CIDR (port 443) and endpoint IPs (port 6443)
// - Allow egress to cluster pods on API and cluster ports for Raft communication
//
// Note: NetworkPolicies operate at L3/L4 and cannot restrict HTTP paths. The operator
// uses specific OpenBao API endpoints (GET /v1/sys/health, PUT /v1/sys/init, etc.),
// but endpoint-level access control is enforced by OpenBao's authentication.
func buildNetworkPolicy(cluster *openbaov1alpha1.OpenBaoCluster, apiServerInfo *apiServerInfo, operatorNamespace string) (*networkingv1.NetworkPolicy, error) {
	labels := infraLabels(cluster)
	podSelector := podSelectorLabels(cluster)

	// Allow ingress from pods within the same cluster
	clusterPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: podSelector,
		},
	}

	// Allow ingress from the OpenBao operator pods on port 8200.
	// The operator uses these OpenBao API endpoints:
	// - GET /v1/sys/health (init manager, upgrade manager)
	// - PUT /v1/sys/init (init manager, standard clusters only)
	// - PUT /v1/sys/step-down (upgrade manager)
	// The operator pods are in a different namespace, so we use both NamespaceSelector
	// and PodSelector to match pods in the operator namespace with the operator labels.
	// The namespace selector uses the standard Kubernetes namespace name label.
	// The operator pods are labeled with app.kubernetes.io/name=openbao-operator and
	// app.kubernetes.io/component=controller (not control-plane=controller-manager).
	operatorPeer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": operatorNamespace,
			},
		},
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.LabelAppName: constants.LabelValueAppNameOpenBaoOperator,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      constants.LabelAppComponent,
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"controller", "provisioner"},
				},
			},
		},
	}

	// DNS egress rule - allow UDP and TCP on port 53
	dnsPort := intstr.FromInt(53)
	dnsProtocolUDP := corev1.ProtocolUDP
	dnsProtocolTCP := corev1.ProtocolTCP
	dnsNamespace := "kube-system"
	if cluster.Spec.Network != nil && cluster.Spec.Network.DNSNamespace != "" {
		dnsNamespace = cluster.Spec.Network.DNSNamespace
	}

	// Kubernetes API egress ports
	kubernetesAPIPort443 := intstr.FromInt(443)   // Service IP port
	kubernetesAPIPort6443 := intstr.FromInt(6443) // Direct endpoint port

	// Cluster communication egress - allow communication to other cluster pods
	apiPort := intstr.FromInt(constants.PortAPI)
	clusterPort := intstr.FromInt(constants.PortCluster)

	// Build egress rules dynamically based on detected API server information
	egressRules := []networkingv1.NetworkPolicyEgressRule{
		{
			// Allow DNS egress for service discovery
			// DNS can be in kube-system namespace or as a system service
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": dnsNamespace,
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &dnsProtocolUDP,
					Port:     &dnsPort,
				},
				{
					Protocol: &dnsProtocolTCP,
					Port:     &dnsPort,
				},
			},
		},
	}

	// Add service network CIDR rule if detected (works for all cluster types)
	if apiServerInfo != nil && apiServerInfo.ServiceNetworkCIDR != "" {
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			// Allow egress to Kubernetes API server via service network (port 443).
			// This works for managed clusters (EKS, GKE, AKS) where the API server
			// is external and accessed via the service IP.
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: apiServerInfo.ServiceNetworkCIDR,
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &kubernetesAPIPort443,
				},
			},
		})
	}

	// Add endpoint IP rules if detected (works for self-managed clusters)
	if apiServerInfo != nil && len(apiServerInfo.EndpointIPs) > 0 {
		for _, endpointIP := range apiServerInfo.EndpointIPs {
			endpointCIDR, err := ipToSingleHostCIDR(endpointIP)
			if err != nil {
				return nil, err
			}
			egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
				// Allow egress to Kubernetes API server endpoint IPs (port 6443).
				// This works for self-managed clusters (k3d, kubeadm) where the API server
				// runs on control plane nodes with specific IPs.
				To: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: endpointCIDR,
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &kubernetesAPIPort6443,
					},
				},
			})
		}
	}

	// SECURITY: We no longer use permissive fallback rules. API server detection
	// must succeed before building the NetworkPolicy. This is enforced in ensureNetworkPolicy.
	// If we reach here without API server info, it's a programming error.
	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return nil, fmt.Errorf("API server information is required but not provided")
	}

	// Add cluster pod communication rule
	egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
		// Allow egress to cluster pods for Raft communication
		To: []networkingv1.NetworkPolicyPeer{clusterPeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &clusterPort,
			},
		},
	})

	// Merge user-provided egress rules (append after operator-managed rules)
	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		egressRules = append(egressRules, cluster.Spec.Network.EgressRules...)
	}

	// Build operator-managed ingress rules
	ingressRules := buildNetworkPolicyIngressRules(cluster, clusterPeer, operatorPeer, apiPort, clusterPort)

	// Merge user-provided ingress rules (append after operator-managed rules)
	if cluster.Spec.Network != nil && len(cluster.Spec.Network.IngressRules) > 0 {
		ingressRules = append(ingressRules, cluster.Spec.Network.IngressRules...)
	}

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: podSelector,
				// Exclude backup and restore job pods from this NetworkPolicy.
				// These jobs have different network requirements (e.g., access to object storage)
				// and should be managed by separate NetworkPolicies if restrictions are needed.
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "openbao.org/component",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"backup", "restore", "upgrade-snapshot"},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress:  egressRules,
		},
	}

	return networkPolicy, nil
}

func buildJobNetworkPolicy(cluster *openbaov1alpha1.OpenBaoCluster, apiServerInfo *apiServerInfo) (*networkingv1.NetworkPolicy, error) {
	labels := infraLabels(cluster)

	dnsPort := intstr.FromInt(53)
	dnsProtocolUDP := corev1.ProtocolUDP
	dnsProtocolTCP := corev1.ProtocolTCP
	kubernetesAPIPort443 := intstr.FromInt(443)
	kubernetesAPIPort6443 := intstr.FromInt(6443)
	openBaoAPIPort := intstr.FromInt(constants.PortAPI)

	openBaoPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: infraLabels(cluster),
		},
	}

	dnsNamespace := "kube-system"
	if cluster.Spec.Network != nil && cluster.Spec.Network.DNSNamespace != "" {
		dnsNamespace = cluster.Spec.Network.DNSNamespace
	}

	egressRules := []networkingv1.NetworkPolicyEgressRule{
		{
			// Allow DNS egress for name resolution.
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": dnsNamespace,
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &dnsProtocolUDP,
					Port:     &dnsPort,
				},
				{
					Protocol: &dnsProtocolTCP,
					Port:     &dnsPort,
				},
			},
		},
		{
			// Allow egress to OpenBao API (to fetch/restore snapshots, etc.).
			To: []networkingv1.NetworkPolicyPeer{openBaoPeer},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &openBaoAPIPort,
				},
			},
		},
	}

	// Add service network CIDR rule if detected.
	if apiServerInfo != nil && apiServerInfo.ServiceNetworkCIDR != "" {
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: apiServerInfo.ServiceNetworkCIDR,
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &kubernetesAPIPort443,
				},
			},
		})
	}

	// Add endpoint IP rules if detected (self-managed clusters).
	if apiServerInfo != nil && len(apiServerInfo.EndpointIPs) > 0 {
		for _, endpointIP := range apiServerInfo.EndpointIPs {
			endpointCIDR, err := ipToSingleHostCIDR(endpointIP)
			if err != nil {
				return nil, err
			}
			egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: endpointCIDR,
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &kubernetesAPIPort6443,
					},
				},
			})
		}
	}

	// Development profile convenience: if the user didn't provide explicit egress rules,
	// allow common HTTPS egress for backup/restore targets.
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		httpsPort := intstr.FromInt(443)
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"},
				},
				{
					IPBlock: &networkingv1.IPBlock{CIDR: "::/0"},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &httpsPort,
				},
			},
		})
	}

	// Respect user-provided egress rules as additional allowances.
	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		egressRules = append(egressRules, cluster.Spec.Network.EgressRules...)
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobNetworkPolicyName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					constants.LabelOpenBaoCluster: cluster.Name,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.LabelOpenBaoComponent,
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"backup", "restore", "upgrade-snapshot"},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			// Default deny all ingress to job pods.
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
			Egress:  egressRules,
		},
	}, nil
}

// networkPolicyName returns the name for the NetworkPolicy resource.
func networkPolicyName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-network-policy"
}

func jobNetworkPolicyName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-jobs-network-policy"
}

// ensureGatewayCAConfigMap creates or updates a ConfigMap containing the OpenBaoCluster CA certificate.
// This ConfigMap is required for BackendTLSPolicy when using Traefik Gateway API, as Traefik only
// supports ConfigMap references for CA certificates (not Secrets).
//
// The ConfigMap is automatically kept in sync with the CA Secret and is deleted when Gateway is disabled.
func (m *Manager) ensureGatewayCAConfigMap(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	gatewayCfg := cluster.Spec.Gateway
	enabled := gatewayCfg != nil && gatewayCfg.Enabled

	configMapName := cluster.Name + constants.SuffixTLSCA

	if !enabled {
		// If Gateway is disabled and ConfigMap exists, delete it
		configMap := &corev1.ConfigMap{}
		err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      configMapName,
		}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil // Already deleted, nothing to do
			}
			return fmt.Errorf("failed to get Gateway CA ConfigMap %s/%s: %w", cluster.Namespace, configMapName, err)
		}

		logger.Info("Gateway disabled; deleting CA ConfigMap", "configmap", configMapName)
		if deleteErr := m.client.Delete(ctx, configMap); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
			return fmt.Errorf("failed to delete Gateway CA ConfigMap %s/%s: %w", cluster.Namespace, configMapName, deleteErr)
		}
		return nil
	}

	// Gateway is enabled - ensure ConfigMap exists with CA certificate
	// First, get the CA Secret to extract the certificate
	caSecretName := cluster.Name + constants.SuffixTLSCA
	caSecret := &corev1.Secret{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      caSecretName,
	}, caSecret); err != nil {
		if apierrors.IsNotFound(err) {
			// CA Secret doesn't exist yet (TLS might not be enabled or not yet created)
			// Log and return - this will be retried on next reconciliation
			logger.V(1).Info("CA Secret not found; skipping Gateway CA ConfigMap creation", "secret", caSecretName)
			return nil
		}
		if apierrors.IsForbidden(err) || strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			// RBAC might not be ready yet (multi-tenant mode race condition)
			// Log and return - this will be retried on next reconciliation execution
			logger.V(1).Info("CA Secret access forbidden (likely waiting for RBAC); skipping Gateway CA ConfigMap creation", "secret", caSecretName)
			return nil
		}
		return fmt.Errorf("failed to get CA Secret %s/%s: %w", cluster.Namespace, caSecretName, err)
	}

	// Extract CA certificate from Secret
	caCertPEM, ok := caSecret.Data["ca.crt"]
	if !ok || len(caCertPEM) == 0 {
		return fmt.Errorf("CA Secret %s/%s missing 'ca.crt' key", cluster.Namespace, caSecretName)
	}

	// Convert []byte to string for ConfigMap data
	caCertString := string(caCertPEM)

	// Use SSA to create or update the ConfigMap
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Data: map[string]string{
			"ca.crt": caCertString,
		},
	}

	if err := m.applyResource(ctx, configMap, cluster); err != nil {
		return fmt.Errorf("failed to ensure Gateway CA ConfigMap %s/%s: %w", cluster.Namespace, configMapName, err)
	}

	return nil
}

// deleteServiceIfExists deletes the Service with the given namespace/name if it exists.
func (m *Manager) deleteServiceIfExists(ctx context.Context, namespace, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}

	service := &corev1.Service{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, service)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := m.client.Delete(ctx, service); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}
