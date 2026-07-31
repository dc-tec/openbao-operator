package hardenedcontract_test

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
	hardenedfixtures "github.com/dc-tec/openbao-operator/test/fixtures/hardenedcontract"
)

func TestEvaluateOpenBaoCluster_HardenedContractViolations(t *testing.T) {
	for _, fixture := range hardenedfixtures.Fixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			cluster := hardenedfixtures.NewValidCluster("default", "fixture")
			if fixture.Configure != nil {
				fixture.Configure(cluster)
			}

			violation := hardenedcontract.EvaluateOpenBaoCluster(cluster)
			if fixture.RuntimeRule == "" {
				if violation != nil {
					t.Fatalf("EvaluateOpenBaoCluster() = %#v, want nil", violation)
				}
				return
			}
			if violation == nil {
				t.Fatalf("EvaluateOpenBaoCluster() = nil, want rule %q", fixture.RuntimeRule)
			}
			if violation.Reason != constants.ReasonSecurityViolation || violation.Rule != fixture.RuntimeRule {
				t.Fatalf(
					"EvaluateOpenBaoCluster() = %#v, want SecurityViolation rule %q",
					violation,
					fixture.RuntimeRule,
				)
			}
		})
	}
}

func TestNetworkPolicyPeerExplicit(t *testing.T) {
	tests := []struct {
		name string
		peer networkingv1.NetworkPolicyPeer
		want bool
	}{
		{name: "empty", peer: networkingv1.NetworkPolicyPeer{}, want: false},
		{
			name: "namespace selector",
			peer: networkingv1.NetworkPolicyPeer{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "apps"},
				},
			},
			want: true,
		},
		{
			name: "empty namespace selector",
			peer: networkingv1.NetworkPolicyPeer{
				NamespaceSelector: &metav1.LabelSelector{},
			},
			want: false,
		},
		{
			name: "specific ipblock",
			peer: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: "203.0.113.10/32"},
			},
			want: true,
		},
		{
			name: "wildcard ipblock",
			peer: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"},
			},
			want: false,
		},
		{
			name: "link-local ipblock",
			peer: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: "169.254.169.254/32"},
			},
			want: false,
		},
		{
			name: "cidr containing loopback",
			peer: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: "126.0.0.0/7"},
			},
			want: false,
		},
		{
			name: "cidr containing ipv4 link-local",
			peer: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: "169.0.0.0/8"},
			},
			want: false,
		},
		{
			name: "cidr containing ipv6 link-local",
			peer: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: "fe00::/7"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hardenedcontract.NetworkPolicyPeerExplicit(tt.peer); got != tt.want {
				t.Fatalf("NetworkPolicyPeerExplicit() = %v, want %v", got, tt.want)
			}
		})
	}
}
