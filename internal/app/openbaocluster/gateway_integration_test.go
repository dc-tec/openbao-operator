package openbaocluster

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEvaluateGatewayIntegrationRouteStatus(t *testing.T) {
	t.Parallel()

	accepted := metav1.Condition{
		Type:   string(gatewayv1.GatewayClassConditionStatusAccepted),
		Status: metav1.ConditionTrue,
		Reason: string(gatewayv1.GatewayClassReasonAccepted),
	}
	programmed := metav1.Condition{
		Type:   string(gatewayv1.GatewayConditionProgrammed),
		Status: metav1.ConditionTrue,
		Reason: "Programmed",
	}
	routeAccepted := metav1.Condition{
		Type:               string(gatewayv1.RouteConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.RouteReasonAccepted),
		ObservedGeneration: 1,
	}
	routeResolved := metav1.Condition{
		Type:               string(gatewayv1.RouteConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.RouteReasonResolvedRefs),
		ObservedGeneration: 1,
	}

	tests := []struct {
		name       string
		conditions []metav1.Condition
		include    bool
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "missing route is pending",
			wantStatus: metav1.ConditionUnknown,
			wantReason: constants.ReasonGatewayRoutePending,
		},
		{
			name: "rejected route is false",
			conditions: []metav1.Condition{
				{
					Type:               string(gatewayv1.RouteConditionAccepted),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonNotAllowedByListeners),
					ObservedGeneration: 1,
				},
				routeResolved,
			},
			include:    true,
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonGatewayRouteNotAccepted,
		},
		{
			name: "unresolved references are false",
			conditions: []metav1.Condition{
				routeAccepted,
				{
					Type:               string(gatewayv1.RouteConditionResolvedRefs),
					Status:             metav1.ConditionFalse,
					Reason:             string(gatewayv1.RouteReasonBackendNotFound),
					ObservedGeneration: 1,
				},
			},
			include:    true,
			wantStatus: metav1.ConditionFalse,
			wantReason: constants.ReasonGatewayRouteReferencesUnresolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("add OpenBao API to scheme: %v", err)
			}
			if err := gatewayv1.Install(scheme); err != nil {
				t.Fatalf("add Gateway API to scheme: %v", err)
			}

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{Enabled: true},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled: true,
						GatewayRef: openbaov1alpha1.GatewayReference{
							Name:      "shared-gateway",
							Namespace: "gateway-system",
						},
						Hostname: "bao.example.test",
					},
				},
			}
			disabled := false
			cluster.Spec.Gateway.BackendTLS = &openbaov1alpha1.BackendTLSConfig{Enabled: &disabled}
			gateway := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-gateway", Namespace: "gateway-system"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "shared-class",
					Listeners: []gatewayv1.Listener{{
						Name: "https", Protocol: gatewayv1.HTTPSProtocolType, Port: 443,
					}},
				},
				Status: gatewayv1.GatewayStatus{Conditions: []metav1.Condition{programmed}},
			}
			gatewayClass := &gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-class"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: "example.net/gateway-controller",
				},
				Status: gatewayv1.GatewayClassStatus{
					Conditions: []metav1.Condition{accepted},
					SupportedFeatures: []gatewayv1.SupportedFeature{{
						Name: "HTTPRoute",
					}},
				},
			}
			objects := []client.Object{gateway, gatewayClass}
			if tt.include {
				gatewayNamespace := gatewayv1.Namespace("gateway-system")
				objects = append(objects, &gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{Name: "example-httproute", Namespace: "default", Generation: 1},
					Status: gatewayv1.HTTPRouteStatus{
						RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{{
							ParentRef: gatewayv1.ParentReference{
								Name:      "shared-gateway",
								Namespace: &gatewayNamespace,
							},
							ControllerName: "example.net/gateway-controller",
							Conditions:     tt.conditions,
						}}},
					},
				})
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
			result := EvaluateGatewayIntegration(t.Context(), StatusIntegrationDependencies{
				Client:    k8sClient,
				APIReader: k8sClient,
				Scheme:    scheme,
			}, cluster)
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Fatalf(
					"EvaluateGatewayIntegration() = (%s, %s), want (%s, %s): %s",
					result.Status,
					result.Reason,
					tt.wantStatus,
					tt.wantReason,
					result.Message,
				)
			}
		})
	}
}
