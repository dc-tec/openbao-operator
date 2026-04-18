package networking

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// buildNetworkPolicy constructs a NetworkPolicy for the given OpenBaoCluster.
func buildNetworkPolicy(cluster *openbaov1alpha1.OpenBaoCluster, apiServerInfo *apiServerInfo, operatorNamespace string) (*networkingv1.NetworkPolicy, error) {
	labels := infraLabels(cluster)
	podSelector := podSelectorLabels(cluster)

	clusterPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: podSelector,
		},
	}

	operatorPeer := networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": operatorNamespace,
			},
		},
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.LabelAppName: constants.LabelValueAppNameOpenBaoOperator,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      constants.LabelAppComponent,
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"controller", "provisioner"},
				},
			},
		},
	}

	kubernetesAPIPort443 := intstr.FromInt(443)
	kubernetesAPIPort6443 := intstr.FromInt(6443)
	apiPort := intstr.FromInt(constants.PortAPI)
	clusterPort := intstr.FromInt(constants.PortCluster)

	egressRules, err := buildDNSEgressRules(cluster)
	if err != nil {
		return nil, err
	}

	if apiServerInfo != nil && apiServerInfo.ServiceNetworkCIDR != "" {
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: apiServerInfo.ServiceNetworkCIDR,
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &kubernetesAPIPort443,
				},
			},
		})
	}

	if apiServerInfo != nil && len(apiServerInfo.EndpointIPs) > 0 {
		for _, endpointIP := range apiServerInfo.EndpointIPs {
			endpointCIDR, err := ipToSingleHostCIDR(endpointIP)
			if err != nil {
				return nil, err
			}
			egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: endpointCIDR,
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &kubernetesAPIPort6443,
					},
				},
			})
		}
	}

	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return nil, fmt.Errorf("API server information is required but not provided")
	}

	egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{clusterPeer},
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
	})

	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		egressRules = append(egressRules, cluster.Spec.Network.EgressRules...)
	}

	ingressRules := buildNetworkPolicyIngressRules(cluster, clusterPeer, operatorPeer, apiPort, clusterPort)
	if cluster.Spec.Network != nil && len(cluster.Spec.Network.IngressRules) > 0 {
		ingressRules = append(ingressRules, cluster.Spec.Network.IngressRules...)
	}

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: podSelector,
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "openbao.org/component",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"backup", "restore", "upgrade-snapshot"},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress:  egressRules,
		},
	}

	return networkPolicy, nil
}

func buildJobNetworkPolicy(cluster *openbaov1alpha1.OpenBaoCluster, apiServerInfo *apiServerInfo) (*networkingv1.NetworkPolicy, error) {
	labels := infraLabels(cluster)

	kubernetesAPIPort443 := intstr.FromInt(443)
	kubernetesAPIPort6443 := intstr.FromInt(6443)
	openBaoAPIPort := intstr.FromInt(constants.PortAPI)

	openBaoPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: infraLabels(cluster),
		},
	}

	egressRules, err := buildDNSEgressRules(cluster)
	if err != nil {
		return nil, err
	}
	egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{openBaoPeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &openBaoAPIPort,
			},
		},
	})

	if apiServerInfo != nil && apiServerInfo.ServiceNetworkCIDR != "" {
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: apiServerInfo.ServiceNetworkCIDR,
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &kubernetesAPIPort443,
				},
			},
		})
	}

	if apiServerInfo != nil && len(apiServerInfo.EndpointIPs) > 0 {
		for _, endpointIP := range apiServerInfo.EndpointIPs {
			endpointCIDR, err := ipToSingleHostCIDR(endpointIP)
			if err != nil {
				return nil, err
			}
			egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: endpointCIDR,
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
						Port:     &kubernetesAPIPort6443,
					},
				},
			})
		}
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment &&
		(cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0) {
		httpsPort := intstr.FromInt(443)
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"},
				},
				{
					IPBlock: &networkingv1.IPBlock{CIDR: "::/0"},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &httpsPort,
				},
			},
		})
	}

	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		egressRules = append(egressRules, cluster.Spec.Network.EgressRules...)
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobNetworkPolicyName(cluster),
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					constants.LabelOpenBaoCluster: cluster.Name,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.LabelOpenBaoComponent,
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"backup", "restore", "upgrade-snapshot"},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
			Egress:  egressRules,
		},
	}, nil
}

// networkPolicyName returns the name for the NetworkPolicy resource.
func networkPolicyName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-network-policy"
}

func jobNetworkPolicyName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-jobs-network-policy"
}
