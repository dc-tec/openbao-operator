package networking

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestValidateGatewayIntegration(t *testing.T) {
	t.Parallel()

	newGateway := func(listeners []gatewayv1.Listener, conditions ...metav1.Condition) *gatewayv1.Gateway {
		return &gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "shared-gateway",
				Namespace: "gateway-system",
			},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: "shared-class",
				Listeners:        listeners,
			},
			Status: gatewayv1.GatewayStatus{
				Conditions: conditions,
			},
		}
	}

	newGatewayClass := func(features []gatewayv1.FeatureName, conditions ...metav1.Condition) *gatewayv1.GatewayClass {
		supportedFeatures := make([]gatewayv1.SupportedFeature, 0, len(features))
		for _, feature := range features {
			supportedFeatures = append(supportedFeatures, gatewayv1.SupportedFeature{Name: feature})
		}
		return &gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "shared-class",
			},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.net/gateway-controller",
			},
			Status: gatewayv1.GatewayClassStatus{
				Conditions:        conditions,
				SupportedFeatures: supportedFeatures,
			},
		}
	}

	acceptedTrue := metav1.Condition{
		Type:   string(gatewayv1.GatewayClassConditionStatusAccepted),
		Status: metav1.ConditionTrue,
		Reason: string(gatewayv1.GatewayClassReasonAccepted),
	}
	supportedVersionTrue := metav1.Condition{
		Type:   string(gatewayv1.GatewayClassConditionStatusSupportedVersion),
		Status: metav1.ConditionTrue,
		Reason: string(gatewayv1.GatewayClassReasonSupportedVersion),
	}
	programmedTrue := metav1.Condition{
		Type:   string(gatewayv1.GatewayConditionProgrammed),
		Status: metav1.ConditionTrue,
		Reason: "Programmed",
	}
	routeAcceptedTrue := metav1.Condition{
		Type:               string(gatewayv1.RouteConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.RouteReasonAccepted),
		ObservedGeneration: 1,
	}
	routeResolvedRefsTrue := metav1.Condition{
		Type:               string(gatewayv1.RouteConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.RouteReasonResolvedRefs),
		ObservedGeneration: 1,
	}
	newRouteParent := func(
		controllerName gatewayv1.GatewayController,
		listenerName string,
		conditions ...metav1.Condition,
	) gatewayv1.RouteParentStatus {
		gatewayNamespace := gatewayv1.Namespace("gateway-system")
		parentRef := gatewayv1.ParentReference{
			Name:      "shared-gateway",
			Namespace: &gatewayNamespace,
		}
		if listenerName != "" {
			sectionName := gatewayv1.SectionName(listenerName)
			parentRef.SectionName = &sectionName
		}
		return gatewayv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: controllerName,
			Conditions:     conditions,
		}
	}
	newHTTPRoute := func(parents ...gatewayv1.RouteParentStatus) *gatewayv1.HTTPRoute {
		return &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "example-httproute", Namespace: "default", Generation: 1},
			Status: gatewayv1.HTTPRouteStatus{
				RouteStatus: gatewayv1.RouteStatus{Parents: parents},
			},
		}
	}
	newTLSRoute := func(parents ...gatewayv1.RouteParentStatus) *gatewayv1.TLSRoute {
		return &gatewayv1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "example-tlsroute", Namespace: "default", Generation: 1},
			Status: gatewayv1.TLSRouteStatus{
				RouteStatus: gatewayv1.RouteStatus{Parents: parents},
			},
		}
	}

	tlsPassthrough := gatewayv1.TLSModePassthrough

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		objects   []client.Object
		wantError error
	}{
		{
			name:    "disabled gateway returns nil",
			cluster: newMinimalCluster("example", "default"),
		},
		{
			name: "termination path succeeds when class advertises required features",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
				}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute, gatewayFeatureBackendTLSPolicy}, acceptedTrue, supportedVersionTrue),
				newHTTPRoute(newRouteParent("example.net/gateway-controller", "", routeAcceptedTrue, routeResolvedRefsTrue)),
			},
		},
		{
			name: "termination path tolerates class without SupportedVersion condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				disabled := false
				cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
				}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute}, acceptedTrue),
				newHTTPRoute(newRouteParent("example.net/gateway-controller", "", routeAcceptedTrue, routeResolvedRefsTrue)),
			},
		},
		{
			name: "missing managed route is pending",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				disabled := false
				cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{Name: "https", Protocol: gatewayv1.HTTPSProtocolType, Port: 443}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute}, acceptedTrue, supportedVersionTrue),
			},
			wantError: ErrGatewayRoutePending,
		},
		{
			name: "explicit route rejection is false contract",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				disabled := false
				cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{Name: "https", Protocol: gatewayv1.HTTPSProtocolType, Port: 443}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute}, acceptedTrue, supportedVersionTrue),
				newHTTPRoute(newRouteParent("example.net/gateway-controller", "", metav1.Condition{
					Type:               string(gatewayv1.RouteConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonNotAllowedByListeners),
					Message:            "route is not allowed",
					ObservedGeneration: 1,
				}, routeResolvedRefsTrue)),
			},
			wantError: ErrGatewayRouteNotAccepted,
		},
		{
			name: "unresolved route references are false contract",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				disabled := false
				cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{Name: "https", Protocol: gatewayv1.HTTPSProtocolType, Port: 443}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute}, acceptedTrue, supportedVersionTrue),
				newHTTPRoute(newRouteParent("example.net/gateway-controller", "", routeAcceptedTrue, metav1.Condition{
					Type:               string(gatewayv1.RouteConditionResolvedRefs),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonBackendNotFound),
					Message:            "backend Service was not found",
					ObservedGeneration: 1,
				})),
			},
			wantError: ErrGatewayRouteReferencesUnresolved,
		},
		{
			name: "tls route uses relevant listener and controller parent status",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:        true,
					TLSPassthrough: true,
					ListenerName:   "tls",
					Hostname:       "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name: "tls", Protocol: gatewayv1.TLSProtocolType, Port: 443,
					TLS: &gatewayv1.ListenerTLSConfig{Mode: &tlsPassthrough},
				}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureTLSRoute}, acceptedTrue, supportedVersionTrue),
				newTLSRoute(
					newRouteParent("other.example/controller", "tls", routeAcceptedTrue, routeResolvedRefsTrue),
					newRouteParent("example.net/gateway-controller", "tls", routeAcceptedTrue, routeResolvedRefsTrue),
				),
			},
		},
		{
			name: "listener mismatch is explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:        true,
					TLSPassthrough: true,
					Hostname:       "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
				}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureTLSRoute}, acceptedTrue, supportedVersionTrue),
			},
			wantError: ErrGatewayListenerIncompatible,
		},
		{
			name: "missing supportedFeatures becomes unknown contract",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				disabled := false
				cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
				}}, programmedTrue),
				newGatewayClass(nil, acceptedTrue, supportedVersionTrue),
			},
			wantError: ErrGatewayCapabilitiesUnknown,
		},
		{
			name: "missing tlsroute feature is explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:        true,
					TLSPassthrough: true,
					Hostname:       "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name:     "tls",
					Protocol: gatewayv1.TLSProtocolType,
					Port:     443,
					TLS:      &gatewayv1.ListenerTLSConfig{Mode: &tlsPassthrough},
				}}, programmedTrue),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute}, acceptedTrue, supportedVersionTrue),
			},
			wantError: ErrGatewayFeatureUnsupported,
		},
		{
			name: "gateway programmed false is explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newMinimalCluster("example", "default")
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:  true,
					Hostname: "bao.example.test",
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name:      "shared-gateway",
						Namespace: "gateway-system",
					},
				}
				disabled := false
				cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
				return cluster
			}(),
			objects: []client.Object{
				newGateway([]gatewayv1.Listener{{
					Name:     "https",
					Protocol: gatewayv1.HTTPSProtocolType,
					Port:     443,
				}}, metav1.Condition{
					Type:   string(gatewayv1.GatewayConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: "ListenersNotReady",
				}),
				newGatewayClass([]gatewayv1.FeatureName{gatewayFeatureHTTPRoute}, acceptedTrue, supportedVersionTrue),
			},
			wantError: ErrGatewayNotProgrammed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(testScheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			k8sClient := builder.Build()
			manager := NewManagerWithReader(k8sClient, k8sClient, testScheme, "openbao-operator-system", "")
			err := manager.ValidateGatewayIntegration(context.Background(), tt.cluster)
			if tt.wantError == nil {
				if err != nil {
					t.Fatalf("ValidateGatewayIntegration() error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("ValidateGatewayIntegration() error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

type forbiddenGatewayReader struct {
	client.Reader
}

func (r forbiddenGatewayReader) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	switch obj.(type) {
	case *gatewayv1.Gateway:
		return apierrors.NewForbidden(
			schema.GroupResource{Group: gatewayv1.GroupVersion.Group, Resource: "gateways"},
			key.Name,
			errors.New("rbac denied"),
		)
	default:
		return r.Reader.Get(ctx, key, obj, opts...)
	}
}

func TestValidateGatewayIntegration_ForbiddenGatewayReadIsCapabilitiesUnknown(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("example", "default")
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled:  true,
		Hostname: "bao.example.test",
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name:      "shared-gateway",
			Namespace: "gateway-system",
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	manager := NewManagerWithReader(
		k8sClient,
		forbiddenGatewayReader{Reader: k8sClient},
		testScheme,
		"openbao-operator-system",
		"",
	)

	err := manager.ValidateGatewayIntegration(context.Background(), cluster)
	if !errors.Is(err, ErrGatewayCapabilitiesUnknown) {
		t.Fatalf("ValidateGatewayIntegration() error = %v, want %v", err, ErrGatewayCapabilitiesUnknown)
	}
}

func TestValidateRouteParentStatusPending(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("example", "default")
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled: true,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name:      "shared-gateway",
			Namespace: "gateway-system",
		},
		Hostname: "bao.example.test",
	}
	gatewayClass := &gatewayv1.GatewayClass{
		Spec: gatewayv1.GatewayClassSpec{ControllerName: "example.net/gateway-controller"},
	}
	gatewayNamespace := gatewayv1.Namespace("gateway-system")
	parentRef := gatewayv1.ParentReference{Name: "shared-gateway", Namespace: &gatewayNamespace}
	condition := func(conditionType gatewayv1.RouteConditionType, status metav1.ConditionStatus, generation int64) metav1.Condition {
		return metav1.Condition{
			Type:               string(conditionType),
			Status:             status,
			Reason:             "test",
			ObservedGeneration: generation,
		}
	}
	parent := func(conditions ...metav1.Condition) []gatewayv1.RouteParentStatus {
		return []gatewayv1.RouteParentStatus{{
			ParentRef:      parentRef,
			ControllerName: gatewayClass.Spec.ControllerName,
			Conditions:     conditions,
		}}
	}

	tests := []struct {
		name    string
		parents []gatewayv1.RouteParentStatus
	}{
		{name: "missing parent status"},
		{
			name: "missing Accepted",
			parents: parent(condition(
				gatewayv1.RouteConditionResolvedRefs,
				metav1.ConditionTrue,
				2,
			)),
		},
		{
			name: "Accepted Unknown",
			parents: parent(
				condition(gatewayv1.RouteConditionAccepted, metav1.ConditionUnknown, 2),
				condition(gatewayv1.RouteConditionResolvedRefs, metav1.ConditionTrue, 2),
			),
		},
		{
			name: "stale Accepted",
			parents: parent(
				condition(gatewayv1.RouteConditionAccepted, metav1.ConditionTrue, 1),
				condition(gatewayv1.RouteConditionResolvedRefs, metav1.ConditionTrue, 2),
			),
		},
		{
			name: "missing ResolvedRefs",
			parents: parent(condition(
				gatewayv1.RouteConditionAccepted,
				metav1.ConditionTrue,
				2,
			)),
		},
		{
			name: "ResolvedRefs Unknown",
			parents: parent(
				condition(gatewayv1.RouteConditionAccepted, metav1.ConditionTrue, 2),
				condition(gatewayv1.RouteConditionResolvedRefs, metav1.ConditionUnknown, 2),
			),
		},
		{
			name: "stale ResolvedRefs",
			parents: parent(
				condition(gatewayv1.RouteConditionAccepted, metav1.ConditionTrue, 2),
				condition(gatewayv1.RouteConditionResolvedRefs, metav1.ConditionTrue, 1),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRouteParentStatus(cluster, gatewayClass, "HTTPRoute", 2, tt.parents)
			if !errors.Is(err, ErrGatewayRoutePending) {
				t.Fatalf("validateRouteParentStatus() error = %v, want %v", err, ErrGatewayRoutePending)
			}
		})
	}
}
