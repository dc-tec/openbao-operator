package hardenedcontract

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEvaluateOpenBaoCluster_HardenedContractViolations(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*openbaov1alpha1.OpenBaoCluster)
		want      bool
	}{
		{
			name: "safe baseline",
		},
		{
			name: "tls disabled",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.TLS.Enabled = false
			},
			want: true,
		},
		{
			name: "ambient backup identity",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
					Target: openbaov1alpha1.BackupTarget{
						Bucket: "backups",
					},
				}
			},
			want: true,
		},
		{
			name: "gcs role arn is ambient backup identity",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
					Target: openbaov1alpha1.BackupTarget{
						Provider: "gcs",
						Bucket:   "backups",
						RoleARN:  "arn:aws:iam::123456789012:role/openbao-backup",
					},
				}
			},
			want: true,
		},
		{
			name: "s3 role arn is explicit backup identity",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
					Target: openbaov1alpha1.BackupTarget{
						Provider: "s3",
						Bucket:   "backups",
						RoleARN:  "arn:aws:iam::123456789012:role/openbao-backup",
					},
				}
			},
		},
		{
			name: "empty trusted ingress peer",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					TrustedIngressPeers: []networkingv1.NetworkPolicyPeer{{}},
				}
			},
			want: true,
		},
		{
			name: "wildcard egress",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				port := intstr.FromInt32(443)
				cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
					EgressRules: []networkingv1.NetworkPolicyEgressRule{
						{
							To: []networkingv1.NetworkPolicyPeer{
								{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{Protocol: ptr.To(corev1.ProtocolTCP), Port: &port},
							},
						},
					},
				}
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newSafeHardenedCluster()
			if tt.configure != nil {
				tt.configure(cluster)
			}

			violation := EvaluateOpenBaoCluster(cluster)
			if tt.want {
				if violation == nil || violation.Reason != constants.ReasonSecurityViolation {
					t.Fatalf("EvaluateOpenBaoCluster() = %#v, want SecurityViolation", violation)
				}
				return
			}
			if violation != nil {
				t.Fatalf("EvaluateOpenBaoCluster() = %#v, want nil", violation)
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
			if got := NetworkPolicyPeerExplicit(tt.peer); got != tt.want {
				t.Fatalf("NetworkPolicyPeerExplicit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func newSafeHardenedCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeExternal,
			},
			Network: &openbaov1alpha1.NetworkConfig{
				EgressRules: []networkingv1.NetworkPolicyEgressRule{safeEgressRule()},
			},
		},
	}
}

func safeEgressRule() networkingv1.NetworkPolicyEgressRule {
	port := intstr.FromInt32(443)
	return networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "objectstore"},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: ptr.To(corev1.ProtocolTCP), Port: &port},
		},
	}
}
