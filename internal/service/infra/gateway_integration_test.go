package infra

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
			manager := NewManagerWithReader(k8sClient, k8sClient, testScheme, "openbao-operator-system", "", nil, "")
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
		nil,
		"",
	)

	err := manager.ValidateGatewayIntegration(context.Background(), cluster)
	if !errors.Is(err, ErrGatewayCapabilitiesUnknown) {
		t.Fatalf("ValidateGatewayIntegration() error = %v, want %v", err, ErrGatewayCapabilitiesUnknown)
	}
}
