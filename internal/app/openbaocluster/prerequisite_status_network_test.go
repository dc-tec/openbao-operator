package openbaocluster

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEvaluateAPIServerNetworkConditionPolicy(t *testing.T) {
	scheme := newPrerequisiteStatusTestScheme(t)

	tests := []struct {
		name        string
		envHost     string
		cluster     *openbaov1alpha1.OpenBaoCluster
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "service VIP only is unknown",
			envHost:     "10.43.0.1",
			cluster:     newPrerequisiteStatusTestCluster(),
			wantStatus:  metav1.ConditionUnknown,
			wantReason:  constants.ReasonAPIServerEndpointIPsRecommended,
			wantMessage: "apiServerEndpointIPs",
		},
		{
			name: "explicit endpoint IPs are ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newPrerequisiteStatusTestCluster()
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					APIServerCIDR:        "10.43.0.1/32",
					APIServerEndpointIPs: []string{"192.0.2.10"},
				}
				return cluster
			}(),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonAPIServerNetworkReady,
			wantMessage: "192.0.2.10",
		},
		{
			name: "invalid CIDR is false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newPrerequisiteStatusTestCluster()
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{APIServerCIDR: "not-a-cidr"}
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonAPIServerNetworkConfigurationInvalid,
			wantMessage: "spec.network.apiServerCIDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KUBERNETES_SERVICE_HOST", tt.envHost)
			deps := newPrerequisiteIntegrationTestDependencies(t, scheme)
			result := EvaluateAPIServerNetwork(t.Context(), deps, tt.cluster)
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Errorf("result = status %s, reason %q; want status %s, reason %q", result.Status, result.Reason, tt.wantStatus, tt.wantReason)
			}
			if !contains(result.Message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestEvaluateACMEIntegrationConditionPolicy(t *testing.T) {
	scheme := newPrerequisiteStatusTestScheme(t)

	newACMECluster := func() *openbaov1alpha1.OpenBaoCluster {
		cluster := newPrerequisiteStatusTestCluster()
		cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
		cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"}
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: "transit",
			Transit: &openbaov1alpha1.TransitSealConfig{
				Address:   "https://infra-bao.example",
				KeyName:   "autounseal",
				MountPath: "transit/",
			},
		}
		return cluster
	}

	tests := []struct {
		name        string
		cluster     *openbaov1alpha1.OpenBaoCluster
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "public ACME integration is ready",
			cluster:     newACMECluster(),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonACMEIntegrationReady,
			wantMessage: "prerequisites are satisfied",
		},
		{
			name: "Gateway requires TLS passthrough",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECluster()
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:        true,
					Hostname:       "bao.example.test",
					TLSPassthrough: false,
					GatewayRef:     openbaov1alpha1.GatewayReference{Name: "shared-gateway"},
				}
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonACMEGatewayNotConfiguredForPassthrough,
			wantMessage: "tlsPassthrough=true",
		},
		{
			name: "private ACME domain must resolve in HA",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECluster()
				cluster.Spec.Replicas = 2
				cluster.Status.Initialized = true
				cluster.Spec.TLS.ACME.Domains = []string{"does-not-resolve.invalid"}
				cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{ACMECARoot: "/etc/bao/seal-creds/ca.crt"}
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonACMEDomainNotResolvable,
			wantMessage: "does-not-resolve.invalid",
		},
		{
			name: "private ACME trust bundle is required",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECluster()
				cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{ACMECARoot: "/etc/bao/seal-creds/ca.crt"}
				cluster.Spec.Unseal.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "seal-creds"}
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonPrerequisitesMissing,
			wantMessage: "trust bundle is unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newPrerequisiteIntegrationTestDependencies(t, scheme)
			result := EvaluateACMEIntegration(t.Context(), deps, tt.cluster)
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Errorf("result = status %s, reason %q; want status %s, reason %q", result.Status, result.Reason, tt.wantStatus, tt.wantReason)
			}
			if !contains(result.Message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestEvaluateGatewayIntegrationConditionPolicy(t *testing.T) {
	scheme := newPrerequisiteStatusTestScheme(t)
	accepted := metav1.Condition{Type: string(gatewayv1.GatewayClassConditionStatusAccepted), Status: metav1.ConditionTrue, Reason: string(gatewayv1.GatewayClassReasonAccepted)}
	supportedVersion := metav1.Condition{Type: string(gatewayv1.GatewayClassConditionStatusSupportedVersion), Status: metav1.ConditionTrue, Reason: string(gatewayv1.GatewayClassReasonSupportedVersion)}
	programmed := metav1.Condition{Type: string(gatewayv1.GatewayConditionProgrammed), Status: metav1.ConditionTrue, Reason: "Programmed"}

	tests := []struct {
		name        string
		objects     []client.Object
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name: "integration is ready",
			objects: []client.Object{
				newPrerequisiteGateway(programmed),
				newPrerequisiteGatewayClass([]string{"HTTPRoute"}, accepted, supportedVersion),
				newPrerequisiteHTTPRoute(),
			},
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonGatewayIntegrationReady,
			wantMessage: "Route attachment are ready",
		},
		{
			name: "class may omit SupportedVersion condition",
			objects: []client.Object{
				newPrerequisiteGateway(programmed),
				newPrerequisiteGatewayClass([]string{"HTTPRoute"}, accepted),
				newPrerequisiteHTTPRoute(),
			},
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonGatewayIntegrationReady,
			wantMessage: "Route attachment are ready",
		},
		{
			name: "missing supported features leaves capabilities unknown",
			objects: []client.Object{
				newPrerequisiteGateway(programmed),
				newPrerequisiteGatewayClass(nil, accepted, supportedVersion),
			},
			wantStatus:  metav1.ConditionUnknown,
			wantReason:  constants.ReasonGatewayCapabilitiesUnknown,
			wantMessage: "does not publish status.supportedFeatures",
		},
		{
			name: "unprogrammed Gateway is false",
			objects: []client.Object{
				newPrerequisiteGateway(metav1.Condition{Type: string(gatewayv1.GatewayConditionProgrammed), Status: metav1.ConditionFalse, Reason: "ListenersNotReady"}),
				newPrerequisiteGatewayClass([]string{"HTTPRoute"}, accepted, supportedVersion),
			},
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonGatewayNotProgrammed,
			wantMessage: "not programmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...)
			c := builder.Build()
			deps := StatusIntegrationDependencies{Client: c, APIReader: c, Scheme: scheme}
			result := EvaluateGatewayIntegration(t.Context(), deps, newPrerequisiteGatewayCluster())
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Errorf("result = status %s, reason %q; want status %s, reason %q", result.Status, result.Reason, tt.wantStatus, tt.wantReason)
			}
			if !contains(result.Message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestEvaluateIngressIntegrationConditionPolicy(t *testing.T) {
	scheme := newPrerequisiteStatusTestScheme(t)
	className := "nginx"
	newIngressCluster := func() *openbaov1alpha1.OpenBaoCluster {
		cluster := newPrerequisiteStatusTestCluster()
		cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
			Enabled:       true,
			ClassName:     &className,
			Host:          "bao.example.test",
			ReadinessMode: openbaov1alpha1.IngressReadinessModeLoadBalancerPublished,
		}
		return cluster
	}
	newIngress := func(address string) *networkingv1.Ingress {
		ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"}}
		if address != "" {
			ingress.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{{Hostname: address}}
		}
		return ingress
	}
	ingressClass := &networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: className}}

	tests := []struct {
		name        string
		objects     []client.Object
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "integration is ready",
			objects:     []client.Object{ingressClass, newIngress("lb.example.test")},
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonIngressIntegrationReady,
			wantMessage: "prerequisites are satisfied",
		},
		{
			name:        "missing IngressClass is false",
			objects:     []client.Object{newIngress("")},
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonIngressClassMissing,
			wantMessage: "IngressClass",
		},
		{
			name:        "unpublished load balancer is unknown",
			objects:     []client.Object{ingressClass.DeepCopy(), newIngress("")},
			wantStatus:  metav1.ConditionUnknown,
			wantReason:  constants.ReasonIngressLoadBalancerPending,
			wantMessage: "load balancer address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...)
			c := builder.Build()
			deps := StatusIntegrationDependencies{Client: c, APIReader: c, Scheme: scheme}
			result := EvaluateIngressIntegration(t.Context(), deps, newIngressCluster())
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Errorf("result = status %s, reason %q; want status %s, reason %q", result.Status, result.Reason, tt.wantStatus, tt.wantReason)
			}
			if !contains(result.Message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", result.Message, tt.wantMessage)
			}
		})
	}
}

func newPrerequisiteIntegrationTestDependencies(t *testing.T, scheme *runtime.Scheme) StatusIntegrationDependencies {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return StatusIntegrationDependencies{Client: c, APIReader: c, Scheme: scheme}
}

func newPrerequisiteGatewayCluster() *openbaov1alpha1.OpenBaoCluster {
	cluster := newPrerequisiteStatusTestCluster()
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
}

func newPrerequisiteGateway(conditions ...metav1.Condition) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-gateway", Namespace: "gateway-system"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "shared-class",
			Listeners:        []gatewayv1.Listener{{Name: "https", Protocol: gatewayv1.HTTPSProtocolType, Port: 443}},
		},
		Status: gatewayv1.GatewayStatus{Conditions: conditions},
	}
}

func newPrerequisiteGatewayClass(features []string, conditions ...metav1.Condition) *gatewayv1.GatewayClass {
	supportedFeatures := make([]gatewayv1.SupportedFeature, 0, len(features))
	for _, feature := range features {
		supportedFeatures = append(supportedFeatures, gatewayv1.SupportedFeature{Name: gatewayv1.FeatureName(feature)})
	}
	return &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-class"},
		Spec:       gatewayv1.GatewayClassSpec{ControllerName: "example.net/gateway-controller"},
		Status: gatewayv1.GatewayClassStatus{
			Conditions:        conditions,
			SupportedFeatures: supportedFeatures,
		},
	}
}

func newPrerequisiteHTTPRoute() *gatewayv1.HTTPRoute {
	gatewayNamespace := gatewayv1.Namespace("gateway-system")
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "example-httproute", Namespace: "default", Generation: 1},
		Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{{
			ParentRef:      gatewayv1.ParentReference{Name: "shared-gateway", Namespace: &gatewayNamespace},
			ControllerName: "example.net/gateway-controller",
			Conditions: []metav1.Condition{
				{Type: string(gatewayv1.RouteConditionAccepted), Status: metav1.ConditionTrue, Reason: string(gatewayv1.RouteReasonAccepted), ObservedGeneration: 1},
				{Type: string(gatewayv1.RouteConditionResolvedRefs), Status: metav1.ConditionTrue, Reason: string(gatewayv1.RouteReasonResolvedRefs), ObservedGeneration: 1},
			},
		}}}},
	}
}

func contains(value, substring string) bool {
	return strings.Contains(value, substring)
}
