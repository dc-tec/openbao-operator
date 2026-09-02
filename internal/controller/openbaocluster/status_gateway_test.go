package openbaocluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSetGatewayIntegrationReadyCondition_FastContract(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

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

	newGatewayClass := func(features []string, conditions ...metav1.Condition) *gatewayv1.GatewayClass {
		supportedFeatures := make([]gatewayv1.SupportedFeature, 0, len(features))
		for _, feature := range features {
			supportedFeatures = append(supportedFeatures, gatewayv1.SupportedFeature{Name: gatewayv1.FeatureName(feature)})
		}
		return &gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-class"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "example.net/gateway-controller",
			},
			Status: gatewayv1.GatewayClassStatus{
				Conditions:        conditions,
				SupportedFeatures: supportedFeatures,
			},
		}
	}
	newHTTPRoute := func(conditions ...metav1.Condition) *gatewayv1.HTTPRoute {
		gatewayNamespace := gatewayv1.Namespace("gateway-system")
		return &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "example-httproute", Namespace: "default", Generation: 1},
			Status: gatewayv1.HTTPRouteStatus{
				RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{{
					ParentRef: gatewayv1.ParentReference{
						Name:      "shared-gateway",
						Namespace: &gatewayNamespace,
					},
					ControllerName: "example.net/gateway-controller",
					Conditions:     conditions,
				}}},
			},
		}
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

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		objects       []client.Object
		wantPresent   bool
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "gateway disabled removes condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Status.Conditions = []metav1.Condition{{
					Type:   string(openbaov1alpha1.ConditionGatewayIntegrationReady),
					Status: metav1.ConditionTrue,
					Reason: constants.ReasonGatewayIntegrationReady,
				}}
				return cluster
			}(),
			wantPresent: false,
		},
		{
			name: "gateway integration ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
				newGatewayClass([]string{"HTTPRoute"}, acceptedTrue, supportedVersionTrue),
				newHTTPRoute(routeAcceptedTrue, routeResolvedRefsTrue),
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonGatewayIntegrationReady,
			wantMessageIn: "Route attachment are ready",
		},
		{
			name: "gateway integration ready when class omits SupportedVersion condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
				newGatewayClass([]string{"HTTPRoute"}, acceptedTrue),
				newHTTPRoute(routeAcceptedTrue, routeResolvedRefsTrue),
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonGatewayIntegrationReady,
			wantMessageIn: "Route attachment are ready",
		},
		{
			name: "managed route pending is unknown",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
				newGatewayClass([]string{"HTTPRoute"}, acceptedTrue, supportedVersionTrue),
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    constants.ReasonGatewayRoutePending,
			wantMessageIn: "does not exist yet",
		},
		{
			name: "managed route rejection is false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
				newGatewayClass([]string{"HTTPRoute"}, acceptedTrue, supportedVersionTrue),
				newHTTPRoute(metav1.Condition{
					Type:               string(gatewayv1.RouteConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonNotAllowedByListeners),
					ObservedGeneration: 1,
				}, routeResolvedRefsTrue),
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonGatewayRouteNotAccepted,
			wantMessageIn: "was rejected",
		},
		{
			name: "managed route unresolved references is false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
				newGatewayClass([]string{"HTTPRoute"}, acceptedTrue, supportedVersionTrue),
				newHTTPRoute(routeAcceptedTrue, metav1.Condition{
					Type:               string(gatewayv1.RouteConditionResolvedRefs),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonBackendNotFound),
					ObservedGeneration: 1,
				}),
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonGatewayRouteReferencesUnresolved,
			wantMessageIn: "unresolved references",
		},
		{
			name: "gateway capabilities unknown when class omits features",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
			wantPresent:   true,
			wantStatus:    metav1.ConditionUnknown,
			wantReason:    constants.ReasonGatewayCapabilitiesUnknown,
			wantMessageIn: "does not publish status.supportedFeatures",
		},
		{
			name: "gateway not programmed is false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
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
				newGatewayClass([]string{"HTTPRoute"}, acceptedTrue, supportedVersionTrue),
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonGatewayNotProgrammed,
			wantMessageIn: "not programmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			fakeClient := builder.Build()
			reconciler := &OpenBaoClusterReconciler{
				Client:       fakeClient,
				Applications: newStatusTestApplications(fakeClient, scheme),
			}

			reconciler.setGatewayIntegrationReadyCondition(context.Background(), tt.cluster)
			assertClusterCondition(
				t,
				tt.cluster,
				openbaov1alpha1.ConditionGatewayIntegrationReady,
				tt.wantPresent,
				tt.wantStatus,
				tt.wantReason,
				tt.wantMessageIn,
			)
		})
	}
}
