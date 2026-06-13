package networking

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// buildNetworkPolicyIngressRules constructs the ingress rules for the NetworkPolicy.
// It dynamically includes rules for Gateway controllers based on the cluster configuration.
func buildNetworkPolicyIngressRules(
	cluster *openbaov1alpha1.OpenBaoCluster,
	clusterPeer, operatorPeer networkingv1.NetworkPolicyPeer,
	apiPort, clusterPort intstr.IntOrString,
) []networkingv1.NetworkPolicyIngressRule {
	rules := []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{clusterPeer},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &apiPort,
				},
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &clusterPort,
				},
			},
		},
	}

	if cluster.Spec.Network != nil {
		trustedIngressPorts := []intstr.IntOrString{apiPort}
		if metricsOnlyListenerEnabled(cluster) {
			trustedIngressPorts = append(trustedIngressPorts, intstr.FromInt(int(metricsOnlyListenerPort(cluster))))
		}
		for _, peer := range cluster.Spec.Network.TrustedIngressPeers {
			rules = appendIngressPeerRule(rules, peer, trustedIngressPorts...)
		}
	}

	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{operatorPeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
		},
	})

	backupRestorePeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.LabelOpenBaoCluster: cluster.Name,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "openbao.org/component",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"backup", "restore"},
				},
			},
		},
	}
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{backupRestorePeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
		},
	})

	return rules
}

func appendIngressPeerRule(
	rules []networkingv1.NetworkPolicyIngressRule,
	peer networkingv1.NetworkPolicyPeer,
	ports ...intstr.IntOrString,
) []networkingv1.NetworkPolicyIngressRule {
	networkPolicyPorts := make([]networkingv1.NetworkPolicyPort, 0, len(ports))
	for _, port := range ports {
		networkPolicyPorts = append(networkPolicyPorts, networkingv1.NetworkPolicyPort{
			Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
			Port:     &port,
		})
	}
	return append(rules, networkingv1.NetworkPolicyIngressRule{
		From:  []networkingv1.NetworkPolicyPeer{peer},
		Ports: networkPolicyPorts,
	})
}

func dnsNamespaceForCluster(cluster *openbaov1alpha1.OpenBaoCluster) string {
	dnsNamespace := "kube-system"
	if cluster != nil && cluster.Spec.Network != nil && strings.TrimSpace(cluster.Spec.Network.DNSNamespace) != "" {
		dnsNamespace = strings.TrimSpace(cluster.Spec.Network.DNSNamespace)
	}
	return dnsNamespace
}

func buildDNSEgressRules(cluster *openbaov1alpha1.OpenBaoCluster) ([]networkingv1.NetworkPolicyEgressRule, error) {
	dnsPort := intstr.FromInt(53)
	dnsProtocolUDP := corev1.ProtocolUDP
	dnsProtocolTCP := corev1.ProtocolTCP

	ports := []networkingv1.NetworkPolicyPort{
		{
			Protocol: &dnsProtocolUDP,
			Port:     &dnsPort,
		},
		{
			Protocol: &dnsProtocolTCP,
			Port:     &dnsPort,
		},
	}

	rules := []networkingv1.NetworkPolicyEgressRule{
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": dnsNamespaceForCluster(cluster),
						},
					},
				},
			},
			Ports: ports,
		},
	}

	if cluster == nil || cluster.Spec.Network == nil {
		return rules, nil
	}

	for _, rawIP := range cluster.Spec.Network.DNSEndpointIPs {
		if strings.TrimSpace(rawIP) == "" {
			continue
		}

		cidr, err := ipToSingleHostCIDR(rawIP)
		if err != nil {
			return nil, fmt.Errorf("invalid spec.network.dnsEndpointIPs entry %q: %w", rawIP, err)
		}

		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: cidr,
					},
				},
			},
			Ports: ports,
		})
	}

	return rules, nil
}
