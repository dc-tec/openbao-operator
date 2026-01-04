package infra

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestBuildHTTPRouteBackends_ServiceSelectorsDefault(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-httproute-default",
			Namespace: "default",
		},
	}

	port := gatewayv1.PortNumber(constants.PortAPI)
	backends := buildHTTPRouteBackends(cluster, port)

	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}

	if string(backends[0].Name) != externalServiceName(cluster) {
		t.Fatalf("expected backend name %q, got %q", externalServiceName(cluster), backends[0].Name)
	}

	if backends[0].Weight != nil {
		t.Fatalf("expected nil weight for default backend, got %v", *backends[0].Weight)
	}
}

func TestBuildHTTPRouteBackends_GatewayWeightsSteps(t *testing.T) {
	base := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-httproute-weights",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Gateway: &openbaov1alpha1.GatewayConfig{
				Enabled: true,
				GatewayRef: openbaov1alpha1.GatewayReference{
					Name: "traefik-gateway",
				},
				Hostname: "bao.example.local",
			},
			UpdateStrategy: openbaov1alpha1.UpdateStrategy{
				Type: openbaov1alpha1.UpdateStrategyBlueGreen,
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					TrafficStrategy: openbaov1alpha1.BlueGreenTrafficStrategyGatewayWeights,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision:  "blue123",
				GreenRevision: "green456",
			},
		},
	}

	type weights struct {
		blue  int32
		green int32
	}

	tests := []struct {
		name string
		step int32
		want weights
	}{
		{"step 0 initial", 0, weights{blue: 100, green: 0}},
		{"step 1 canary", 1, weights{blue: 90, green: 10}},
		{"step 2 mid", 2, weights{blue: 50, green: 50}},
		{"step 3 final", 3, weights{blue: 0, green: 100}},
		{"step 4 final-plus", 4, weights{blue: 0, green: 100}},
	}

	port := gatewayv1.PortNumber(constants.PortAPI)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := base.DeepCopy()
			cluster.Status.BlueGreen.TrafficStep = tt.step

			backends := buildHTTPRouteBackends(cluster, port)
			if len(backends) != 2 {
				t.Fatalf("expected 2 backends, got %d", len(backends))
			}

			var blueBackend, greenBackend *gatewayv1.HTTPBackendRef
			for i := range backends {
				b := &backends[i]
				switch string(b.Name) {
				case externalServiceNameBlue(cluster):
					blueBackend = b
				case externalServiceNameGreen(cluster):
					greenBackend = b
				}
			}

			if blueBackend == nil || greenBackend == nil {
				t.Fatalf("expected both blue and green backends to be present")
			}

			if blueBackend.Weight == nil || greenBackend.Weight == nil {
				t.Fatalf("expected both backends to have weights, got blue=%v green=%v", blueBackend.Weight, greenBackend.Weight)
			}

			if *blueBackend.Weight != tt.want.blue || *greenBackend.Weight != tt.want.green {
				t.Fatalf("unexpected weights: blue=%d green=%d, want blue=%d green=%d",
					*blueBackend.Weight, *greenBackend.Weight, tt.want.blue, tt.want.green)
			}
		})
	}
}

func TestBuildJobNetworkPolicy_DevelopmentDefaultEgressIncludesHTTPS(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev-job-policy",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileDevelopment,
		},
	}

	policy := buildJobNetworkPolicy(cluster, &apiServerInfo{ServiceNetworkCIDR: "10.0.0.0/16"})

	var found bool
	for _, rule := range policy.Spec.Egress {
		for _, peer := range rule.To {
			if peer.IPBlock == nil || peer.IPBlock.CIDR != "0.0.0.0/0" {
				continue
			}
			found = true
			got := map[int32]bool{}
			for _, port := range rule.Ports {
				if port.Port == nil || port.Port.Type != intstr.Int {
					continue
				}
				got[port.Port.IntVal] = true
			}
			if !got[443] {
				t.Fatalf("expected development job NetworkPolicy to allow TCP egress on port 443")
			}
		}
	}
	if !found {
		t.Fatalf("expected development job NetworkPolicy to include an allow-all (0.0.0.0/0) egress rule")
	}
}
