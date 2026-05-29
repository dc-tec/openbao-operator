package openbaocluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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
			Status: gatewayv1.GatewayClassStatus{
				Conditions:        conditions,
				SupportedFeatures: supportedFeatures,
			},
		}
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
					Reason: ReasonGatewayIntegrationReady,
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
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonGatewayIntegrationReady,
			wantMessageIn: "prerequisites are satisfied",
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
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    ReasonGatewayIntegrationReady,
			wantMessageIn: "prerequisites are satisfied",
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
			wantReason:    ReasonGatewayCapabilitiesUnknown,
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
			wantReason:    ReasonGatewayNotProgrammed,
			wantMessageIn: "not programmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			reconciler := &OpenBaoClusterReconciler{
				Client: builder.Build(),
				ControllerRuntime: ControllerRuntime{
					Scheme: scheme,
				},
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
