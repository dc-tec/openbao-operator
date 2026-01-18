package infra

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestBuildHTTPRoute_DefaultDoesNotSetSectionName(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-httproute-sectionname-default",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Gateway: &openbaov1alpha1.GatewayConfig{
				Enabled: true,
				GatewayRef: openbaov1alpha1.GatewayReference{
					Name:      "traefik-gateway",
					Namespace: "default",
				},
				Hostname: "bao.example.local",
			},
		},
	}

	route := buildHTTPRoute(cluster)
	if route == nil {
		t.Fatalf("expected non-nil HTTPRoute")
	}
	if len(route.Spec.ParentRefs) != 1 {
		t.Fatalf("expected 1 parentRef, got %d", len(route.Spec.ParentRefs))
	}
	if route.Spec.ParentRefs[0].SectionName != nil {
		t.Fatalf("expected nil sectionName by default, got %q", *route.Spec.ParentRefs[0].SectionName)
	}
}

func TestBuildHTTPRoute_ListenerNameSetsSectionName(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-httproute-sectionname",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Gateway: &openbaov1alpha1.GatewayConfig{
				Enabled:      true,
				ListenerName: "websecure",
				GatewayRef: openbaov1alpha1.GatewayReference{
					Name:      "traefik-gateway",
					Namespace: "default",
				},
				Hostname: "bao.example.local",
			},
		},
	}

	route := buildHTTPRoute(cluster)
	if route == nil {
		t.Fatalf("expected non-nil HTTPRoute")
	}
	if len(route.Spec.ParentRefs) != 1 {
		t.Fatalf("expected 1 parentRef, got %d", len(route.Spec.ParentRefs))
	}
	if route.Spec.ParentRefs[0].SectionName == nil {
		t.Fatalf("expected sectionName to be set")
	}
	if string(*route.Spec.ParentRefs[0].SectionName) != "websecure" {
		t.Fatalf("expected sectionName %q, got %q", "websecure", *route.Spec.ParentRefs[0].SectionName)
	}
}

func TestBuildTLSRoute_ListenerNameSetsSectionName(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-tlsroute-sectionname",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Gateway: &openbaov1alpha1.GatewayConfig{
				Enabled:        true,
				TLSPassthrough: true,
				ListenerName:   "websecure",
				GatewayRef: openbaov1alpha1.GatewayReference{
					Name:      "traefik-gateway",
					Namespace: "default",
				},
				Hostname: "bao.example.local",
			},
		},
	}

	route := buildTLSRoute(cluster)
	if route == nil {
		t.Fatalf("expected non-nil TLSRoute")
	}
	if len(route.Spec.ParentRefs) != 1 {
		t.Fatalf("expected 1 parentRef, got %d", len(route.Spec.ParentRefs))
	}
	if route.Spec.ParentRefs[0].SectionName == nil {
		t.Fatalf("expected sectionName to be set")
	}
	if string(*route.Spec.ParentRefs[0].SectionName) != "websecure" {
		t.Fatalf("expected sectionName %q, got %q", "websecure", *route.Spec.ParentRefs[0].SectionName)
	}
}

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

	policy, err := buildJobNetworkPolicy(cluster, &apiServerInfo{ServiceNetworkCIDR: "10.0.0.0/16"})
	if err != nil {
		t.Fatalf("buildJobNetworkPolicy() error: %v", err)
	}

	var foundIPv4, foundIPv6 bool
	for _, rule := range policy.Spec.Egress {
		for _, peer := range rule.To {
			if peer.IPBlock == nil {
				continue
			}

			switch peer.IPBlock.CIDR {
			case "0.0.0.0/0":
				foundIPv4 = true
			case "::/0":
				foundIPv6 = true
			default:
				continue
			}

			got := map[int32]bool{}
			for _, port := range rule.Ports {
				if port.Port == nil || port.Port.Type != intstr.Int {
					continue
				}
				got[port.Port.IntVal] = true
			}
			if !got[443] {
				t.Fatalf("expected development job NetworkPolicy to allow TCP egress on port 443 for %s", peer.IPBlock.CIDR)
			}
		}
	}
	if !foundIPv4 {
		t.Fatalf("expected development job NetworkPolicy to include an allow-all (0.0.0.0/0) egress rule")
	}
	if !foundIPv6 {
		t.Fatalf("expected development job NetworkPolicy to include an allow-all (::/0) egress rule")
	}
}

func TestBuildBackendTLSPolicy_DefaultOnlyMainService(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-policy-default",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
			},
			Gateway: &openbaov1alpha1.GatewayConfig{
				Enabled: true,
				GatewayRef: openbaov1alpha1.GatewayReference{
					Name: "gateway",
				},
				Hostname: "bao.example.com",
			},
		},
	}

	policy := buildBackendTLSPolicy(cluster)
	if policy == nil {
		t.Fatalf("expected non-nil BackendTLSPolicy")
	}

	if len(policy.Spec.TargetRefs) != 1 {
		t.Fatalf("expected 1 target ref, got %d", len(policy.Spec.TargetRefs))
	}

	if string(policy.Spec.TargetRefs[0].Name) != externalServiceName(cluster) {
		t.Fatalf("expected target ref name %q, got %q", externalServiceName(cluster), policy.Spec.TargetRefs[0].Name)
	}
}

func TestBuildBackendTLSPolicy_AfterUpgradeCompleteOnlyMainService(t *testing.T) {
	// After upgrade completes, GreenRevision is cleared, so only main service should be targeted
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-policy-post-upgrade",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
			},
			Gateway: &openbaov1alpha1.GatewayConfig{
				Enabled: true,
				GatewayRef: openbaov1alpha1.GatewayReference{
					Name: "gateway",
				},
				Hostname: "bao.example.com",
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision:  "green456", // After upgrade, BlueRevision = former GreenRevision
				GreenRevision: "",         // GreenRevision is cleared
				Phase:         openbaov1alpha1.PhaseIdle,
			},
		},
	}

	policy := buildBackendTLSPolicy(cluster)
	if policy == nil {
		t.Fatalf("expected non-nil BackendTLSPolicy")
	}

	if string(policy.Spec.TargetRefs[0].Name) != externalServiceName(cluster) {
		t.Fatalf("expected target ref name %q, got %q", externalServiceName(cluster), policy.Spec.TargetRefs[0].Name)
	}
}

func TestBuildNetworkPolicy_DNSNamespace(t *testing.T) {
	tests := []struct {
		name         string
		dnsNamespace string
		want         string
	}{
		{
			name:         "DefaultDNSNamespace",
			dnsNamespace: "",
			want:         "kube-system",
		},
		{
			name:         "CustomDNSNamespace",
			dnsNamespace: "openshift-dns",
			want:         "openshift-dns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Network: &openbaov1alpha1.NetworkConfig{
						DNSNamespace: tt.dnsNamespace,
					},
				},
			}

			// We need a valid API server info to avoid error
			apiInfo := &apiServerInfo{
				ServiceNetworkCIDR: "10.96.0.0/12",
			}

			policy, err := buildNetworkPolicy(cluster, apiInfo, "openbao-operator-system")
			if err != nil {
				t.Fatalf("buildNetworkPolicy() error: %v", err)
			}

			found := false
			for _, rule := range policy.Spec.Egress {
				for _, peer := range rule.To {
					if peer.NamespaceSelector != nil && peer.NamespaceSelector.MatchLabels != nil {
						if val, ok := peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; ok {
							if val == tt.want {
								found = true
								break
							}
						}
					}
				}
				if found {
					break
				}
			}

			if !found {
				t.Errorf("NetworkPolicy egress rule for DNS namespace %q not found", tt.want)
			}
		})
	}
}

func TestBuildJobNetworkPolicy_DNSNamespace(t *testing.T) {
	tests := []struct {
		name         string
		dnsNamespace string
		want         string
	}{
		{
			name:         "DefaultDNSNamespace",
			dnsNamespace: "",
			want:         "kube-system",
		},
		{
			name:         "CustomDNSNamespace",
			dnsNamespace: "openshift-dns",
			want:         "openshift-dns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Network: &openbaov1alpha1.NetworkConfig{
						DNSNamespace: tt.dnsNamespace,
					},
				},
			}

			policy, err := buildJobNetworkPolicy(cluster, &apiServerInfo{ServiceNetworkCIDR: "10.0.0.0/16"})
			if err != nil {
				t.Fatalf("buildJobNetworkPolicy() error: %v", err)
			}

			found := false
			for _, rule := range policy.Spec.Egress {
				for _, peer := range rule.To {
					if peer.NamespaceSelector != nil && peer.NamespaceSelector.MatchLabels != nil {
						if val, ok := peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; ok {
							if val == tt.want {
								found = true
								break
							}
						}
					}
				}
				if found {
					break
				}
			}

			if !found {
				t.Errorf("Job NetworkPolicy egress rule for DNS namespace %q not found", tt.want)
			}
		})
	}
}
