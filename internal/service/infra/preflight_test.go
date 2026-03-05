package infra

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestValidateGatewayPassthroughListener(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		gateway   *gatewayv1.Gateway
		wantErr   bool
		errTarget error
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			wantErr: false,
		},
		{
			name: "gateway disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{Enabled: false},
				},
			},
			wantErr: false,
		},
		{
			name: "gateway enabled but no passthrough",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "gateway missing",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: true,
						GatewayRef:     openbaov1alpha1.GatewayReference{Name: "missing"},
					},
				},
			},
			wantErr:   true,
			errTarget: ErrACMEGatewayNotConfiguredForPassthrough,
		},
		{
			name: "gateway found, valid listener",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: true,
						GatewayRef:     openbaov1alpha1.GatewayReference{Name: "gw"},
					},
				},
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tls",
							Protocol: gatewayv1.TLSProtocolType,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: pointerTo(gatewayv1.TLSModePassthrough),
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "gateway found, no tls listener",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: true,
						GatewayRef:     openbaov1alpha1.GatewayReference{Name: "gw"},
					},
				},
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
						},
					},
				},
			},
			wantErr:   true,
			errTarget: ErrACMEGatewayNotConfiguredForPassthrough,
		},
		{
			name: "gateway found, tls listener but terminate",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: true,
						GatewayRef:     openbaov1alpha1.GatewayReference{Name: "gw"},
					},
				},
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tls",
							Protocol: gatewayv1.TLSProtocolType,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: pointerTo(gatewayv1.TLSModeTerminate),
								Options: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
									gatewayv1.AnnotationKey("example.com/test"): gatewayv1.AnnotationValue("true"),
								},
							},
						},
					},
				},
			},
			wantErr:   true,
			errTarget: ErrACMEGatewayNotConfiguredForPassthrough,
		},
		{
			name: "gateway found, specific listener missing",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: true,
						GatewayRef:     openbaov1alpha1.GatewayReference{Name: "gw"},
						ListenerName:   "custom",
					},
				},
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name:     "other",
							Protocol: gatewayv1.TLSProtocolType,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: pointerTo(gatewayv1.TLSModePassthrough),
							},
						},
					},
				},
			},
			wantErr:   true,
			errTarget: ErrACMEGatewayNotConfiguredForPassthrough,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var initObjs []client.Object
			if tt.gateway != nil {
				initObjs = append(initObjs, tt.gateway)
			}
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(initObjs...).
				WithReturnManagedFields().
				Build()
			mgr := NewManager(c, scheme, "op-ns", "", nil, "")

			err := mgr.validateGatewayPassthroughListener(context.Background(), tt.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGatewayPassthroughListener() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errTarget != nil {
				if !errors.Is(err, tt.errTarget) {
					t.Errorf("validateGatewayPassthroughListener() error = %v, want target %v", err, tt.errTarget)
				}
			}
		})
	}
}

func TestRunACMEPreflight(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		gateway   *gatewayv1.Gateway
		wantErr   bool
		errTarget error
	}{
		{
			name: "ACME disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeOperatorManaged,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "ACME enabled, Gateway passthrough disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
					},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: false,
					},
				},
			},
			wantErr:   true,
			errTarget: ErrACMEGatewayNotConfiguredForPassthrough,
		},
		{
			name: "ACME enabled, Gateway enabled, valid setup",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME: &openbaov1alpha1.ACMEConfig{
							Domains: []string{"example.com"},
						},
					},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled:        true,
						TLSPassthrough: true,
						GatewayRef:     openbaov1alpha1.GatewayReference{Name: "gw"},
					},
				},
			},
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tls",
							Protocol: gatewayv1.TLSProtocolType,
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: pointerTo(gatewayv1.TLSModePassthrough),
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var initObjs []client.Object
			if tt.gateway != nil {
				initObjs = append(initObjs, tt.gateway)
			}
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(initObjs...).
				WithReturnManagedFields().
				Build()
			mgr := NewManager(c, scheme, "op-ns", "", nil, "")

			err := mgr.runACMEPreflight(context.Background(), logr.Discard(), tt.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("runACMEPreflight() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errTarget != nil {
				if !errors.Is(err, tt.errTarget) {
					t.Errorf("runACMEPreflight() error = %v, want target %v", err, tt.errTarget)
				}
			}
		})
	}
}

func pointerTo[T any](v T) *T {
	return &v
}
