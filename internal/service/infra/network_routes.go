package infra

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

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
