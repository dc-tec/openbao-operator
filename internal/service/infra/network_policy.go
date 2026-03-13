package infra

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

func (m *Manager) ensureNetworkPolicy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	name := networkPolicyName(cluster)

	// Detect API server information for NetworkPolicy rules
	// SECURITY: We require API server detection to succeed to enforce least privilege.
	// Falling back to permissive namespace selectors violates Zero Trust principles.
	apiServerInfo, err := m.detectAPIServerInfo(ctx, logger, cluster)
	if err != nil {
		return wrapAPIServerNetworkConfigurationError("primary", err)
	}
	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return wrapAPIServerNetworkConfigurationError("primary", nil)
	}

	desired, err := buildNetworkPolicy(cluster, apiServerInfo, m.operatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to build NetworkPolicy: %w", err)
	}

	// Set TypeMeta for SSA
	desired.TypeMeta = metav1.TypeMeta{
		Kind:       "NetworkPolicy",
		APIVersion: "networking.k8s.io/v1",
	}

	if err := m.applyResource(ctx, desired, cluster); err != nil {
		return fmt.Errorf("failed to ensure NetworkPolicy %s/%s: %w", cluster.Namespace, name, err)
	}

	return nil
}

// ensureJobNetworkPolicy creates or updates a NetworkPolicy that applies to
// backup/restore/upgrade-snapshot Jobs. These pods are excluded from the main
// OpenBao pod NetworkPolicy because they often need different egress (e.g. object
// storage), but they should still run under explicit network constraints.
func (m *Manager) ensureJobNetworkPolicy(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	name := jobNetworkPolicyName(cluster)

	apiServerInfo, err := m.detectAPIServerInfo(ctx, logger, cluster)
	if err != nil {
		return wrapAPIServerNetworkConfigurationError("job", err)
	}
	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return wrapAPIServerNetworkConfigurationError("job", nil)
	}

	desired, err := buildJobNetworkPolicy(cluster, apiServerInfo)
	if err != nil {
		return fmt.Errorf("failed to build Job NetworkPolicy: %w", err)
	}

	desired.TypeMeta = metav1.TypeMeta{
		Kind:       "NetworkPolicy",
		APIVersion: "networking.k8s.io/v1",
	}

	if err := m.applyResource(ctx, desired, cluster); err != nil {
		return fmt.Errorf("failed to ensure Job NetworkPolicy %s/%s: %w", cluster.Namespace, name, err)
	}

	return nil
}

func wrapAPIServerNetworkConfigurationError(policyScope string, cause error) error {
	scope := "OpenBao"
	if strings.TrimSpace(policyScope) == "job" {
		scope = "job"
	}

	msg := fmt.Sprintf(
		"%s NetworkPolicy requires explicit Kubernetes API egress targets. Configure spec.network.apiServerCIDR. "+
			"If your CNI enforces egress on post-DNAT traffic, also configure spec.network.apiServerEndpointIPs with the control-plane endpoint IPs",
		scope,
	)
	if cause != nil {
		return operatorerrors.WrapPermanentConfig(
			fmt.Errorf("%w: %s: %w", ErrAPIServerNetworkConfigurationInvalid, msg, cause),
		)
	}
	return operatorerrors.WrapPermanentConfig(
		fmt.Errorf("%w: %s", ErrAPIServerNetworkConfigurationInvalid, msg),
	)
}

// apiServerInfo contains detected information about the Kubernetes API server
// for use in NetworkPolicy IPBlock rules.
type apiServerInfo struct {
	// ServiceNetworkCIDR is a single-host CIDR that represents the `kubernetes`
	// Service ClusterIP (e.g., "10.43.0.1/32" or "fd00::1/128").
	// This allows least-privilege access to the in-cluster API service VIP on port 443.
	ServiceNetworkCIDR string
	// EndpointIPs are optional explicit API server endpoint IPs (e.g., control plane node IPs)
	// to allow direct access to the API server on port 6443 in locked-down environments.
	EndpointIPs []string
}

func ipToSingleHostCIDR(ip string) (string, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", fmt.Errorf("invalid IP address %q", ip)
	}
	if parsed.To4() != nil {
		return parsed.String() + "/32", nil
	}
	return parsed.String() + "/128", nil
}

func kubernetesServiceIPCIDRFromEnv() (string, bool) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	if host == "" {
		return "", false
	}
	cidr, err := ipToSingleHostCIDR(host)
	if err != nil {
		return "", false
	}
	return cidr, true
}

// detectAPIServerInfo detects the Kubernetes API server information needed for NetworkPolicy rules.
//
// Primary detection uses the in-cluster service VIP injected into the pod environment
// (KUBERNETES_SERVICE_HOST) so it works under namespace-scoped RBAC (single-tenant mode).
//
// API server endpoint IPs are not auto-detected; they can be configured explicitly via
// spec.network.apiServerEndpointIPs if needed.
func (m *Manager) detectAPIServerInfo(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*apiServerInfo, error) {
	info := &apiServerInfo{}
	reader := m.reader
	if reader == nil {
		reader = m.client
	}
	discovery := newAPIServerDiscovery(reader)

	manualCIDRConfigured := false
	if cluster.Spec.Network != nil && strings.TrimSpace(cluster.Spec.Network.APIServerCIDR) != "" {
		rawCIDR := strings.TrimSpace(cluster.Spec.Network.APIServerCIDR)
		_, ipNet, err := net.ParseCIDR(rawCIDR)
		if err != nil {
			return nil, fmt.Errorf("invalid spec.network.apiServerCIDR %q: %w", rawCIDR, err)
		}
		ipNet.IP = ipNet.IP.Mask(ipNet.Mask)
		canonicalCIDR := ipNet.String()
		logger.V(1).Info("Using manually configured API server CIDR", "cidr", canonicalCIDR)
		if canonicalCIDR != rawCIDR {
			logger.V(1).Info("Normalized API server CIDR", "original", rawCIDR, "normalized", canonicalCIDR)
		}
		info.ServiceNetworkCIDR = canonicalCIDR
		manualCIDRConfigured = true
	}

	if cluster.Spec.Network != nil && len(cluster.Spec.Network.APIServerEndpointIPs) > 0 {
		for _, rawIP := range cluster.Spec.Network.APIServerEndpointIPs {
			ip := strings.TrimSpace(rawIP)
			if ip == "" {
				continue
			}
			parsed := net.ParseIP(ip)
			if parsed == nil {
				return nil, fmt.Errorf("invalid spec.network.apiServerEndpointIPs entry %q: must be an IP address", rawIP)
			}
			info.EndpointIPs = append(info.EndpointIPs, parsed.String())
		}

		if len(info.EndpointIPs) > 0 {
			logger.V(1).Info("Using manually configured API server endpoint IPs", "ips", info.EndpointIPs)
		}
	}

	// Primary, RBAC-free detection path: use the service IP injected into the Pod environment.
	// This is the same source used by in-cluster client-go config and works in single-tenant
	// namespace-scoped installs without cross-namespace reads.
	if !manualCIDRConfigured {
		if cidr, ok := kubernetesServiceIPCIDRFromEnv(); ok {
			info.ServiceNetworkCIDR = cidr
			logger.V(1).Info("Using kubernetes Service IP CIDR from environment", "cidr", cidr)
		}
	}

	// Fallback only if env vars are missing/unparseable and no manual CIDR is configured.
	if !manualCIDRConfigured && info.ServiceNetworkCIDR == "" {
		serviceNetworkCIDR, err := discovery.DiscoverServiceNetworkCIDR(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to detect kubernetes Service IP CIDR (env KUBERNETES_SERVICE_HOST missing/unusable): %w. "+
				"Consider configuring spec.network.apiServerCIDR as a fallback", err)
		}
		if serviceNetworkCIDR != "" {
			info.ServiceNetworkCIDR = serviceNetworkCIDR
			logger.V(1).Info("Detected kubernetes service IP CIDR", "cidr", info.ServiceNetworkCIDR)
		}
	}

	// Note: We intentionally do not auto-detect API server endpoint IPs.
	// Some CNI/NetworkPolicy implementations enforce egress rules on post-DNAT traffic, so
	// allowing only the kubernetes service VIP (port 443) may not be sufficient if traffic is
	// evaluated against the backing endpoint IP/port (commonly port 6443).
	//
	// In those environments, users must configure spec.network.apiServerEndpointIPs to add
	// explicit /32 or /128 egress allow rules for the control plane endpoint(s) on port 6443.

	return info, nil
}

// buildNetworkPolicyIngressRules constructs the ingress rules for the NetworkPolicy.
// It dynamically includes rules for Gateway controllers based on the cluster configuration.
func buildNetworkPolicyIngressRules(
	cluster *openbaov1alpha1.OpenBaoCluster,
	clusterPeer, operatorPeer networkingv1.NetworkPolicyPeer,
	apiPort, clusterPort intstr.IntOrString,
) []networkingv1.NetworkPolicyIngressRule {
	rules := []networkingv1.NetworkPolicyIngressRule{
		{
			// Allow ingress from pods within the same cluster
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

	// If Gateway is enabled, allow ingress from the Gateway resource namespace when
	// it differs from the cluster namespace. This is a best-effort default for the
	// common case where the Gateway data plane runs in the same namespace as the
	// referenced Gateway object. For other topologies, users can declare explicit
	// trusted ingress peers via spec.network.trustedIngressPeers.
	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		gatewayNamespace := cluster.Spec.Gateway.GatewayRef.Namespace
		if strings.TrimSpace(gatewayNamespace) == "" {
			// Default to cluster namespace if not specified
			gatewayNamespace = cluster.Namespace
		}

		// Only add a namespace-wide rule when the Gateway resource lives outside the
		// cluster namespace. If it is colocated, clusterPeer still only covers the
		// OpenBao pods themselves, so user-managed ingress controllers should use
		// spec.network.trustedIngressPeers for a more precise allow-list.
		if gatewayNamespace != cluster.Namespace {
			rules = appendIngressPeerRule(
				rules,
				networkingv1.NetworkPolicyPeer{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": gatewayNamespace,
						},
					},
				},
				apiPort,
			)
		}
	}

	if cluster.Spec.Network != nil {
		for _, peer := range cluster.Spec.Network.TrustedIngressPeers {
			rules = appendIngressPeerRule(rules, peer, apiPort)
		}
	}

	// Always allow ingress from OpenBao operator pods on port 8200
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		// Allow ingress from OpenBao operator pods on port 8200.
		// Used for: GET /v1/sys/health, PUT /v1/sys/init, PUT /v1/sys/step-down
		From: []networkingv1.NetworkPolicyPeer{operatorPeer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
		},
	})

	// Allow ingress from backup and restore pods on port 8200.
	// These pods are labeled with openbao.org/cluster=<cluster-name> and
	// openbao.org/component in (backup, restore). They need to access the
	// leader to perform snapshot/restore operations.
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

	// If standard Ingress is enabled, we must allow traffic to the API port.
	// Since Ingress Controllers can run anywhere (and often preserve client IPs),
	// we allow traffic from anywhere on the API port.
	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &apiPort,
				},
			},
			// Empty "From" implies "Allow from anywhere"
			From: []networkingv1.NetworkPolicyPeer{},
		})
	}

	return rules
}

func appendIngressPeerRule(
	rules []networkingv1.NetworkPolicyIngressRule,
	peer networkingv1.NetworkPolicyPeer,
	apiPort intstr.IntOrString,
) []networkingv1.NetworkPolicyIngressRule {
	return append(rules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{peer},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &apiPort,
			},
		},
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

// buildNetworkPolicy constructs a NetworkPolicy for the given OpenBaoCluster.
// The policy enforces:
// - Default deny all ingress traffic
// - Allow ingress from pods within the same cluster (same pod selector labels)
// - Allow ingress from Gateway namespace (if Gateway is enabled and in different namespace)
// - Allow ingress from OpenBao operator pods on port 8200 (for health checks, initialization, upgrades)
// - Allow egress to DNS (port 53 UDP/TCP) for service discovery
// - Allow egress to Kubernetes API server via service network CIDR (port 443) and endpoint IPs (port 6443)
// - Allow egress to cluster pods on API and cluster ports for Raft communication
//
// Note: NetworkPolicies operate at L3/L4 and cannot restrict HTTP paths. The operator
// uses specific OpenBao API endpoints (GET /v1/sys/health, PUT /v1/sys/init, etc.),
// but endpoint-level access control is enforced by OpenBao's authentication.
func buildNetworkPolicy(cluster *openbaov1alpha1.OpenBaoCluster, apiServerInfo *apiServerInfo, operatorNamespace string) (*networkingv1.NetworkPolicy, error) {
	labels := infraLabels(cluster)
	podSelector := podSelectorLabels(cluster)

	// Allow ingress from pods within the same cluster
	clusterPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: podSelector,
		},
	}

	// Allow ingress from the OpenBao operator pods on port 8200.
	// The operator uses these OpenBao API endpoints:
	// - GET /v1/sys/health (init manager, upgrade manager)
	// - PUT /v1/sys/init (init manager, standard clusters only)
	// - PUT /v1/sys/step-down (upgrade manager)
	// The operator pods are in a different namespace, so we use both NamespaceSelector
	// and PodSelector to match pods in the operator namespace with the operator labels.
	// The namespace selector uses the standard Kubernetes namespace name label.
	// The operator pods are labeled with app.kubernetes.io/name=openbao-operator and
	// app.kubernetes.io/component=controller (not control-plane=controller-manager).
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

	// Kubernetes API egress ports
	kubernetesAPIPort443 := intstr.FromInt(443)   // Service IP port
	kubernetesAPIPort6443 := intstr.FromInt(6443) // Direct endpoint port

	// Cluster communication egress - allow communication to other cluster pods
	apiPort := intstr.FromInt(constants.PortAPI)
	clusterPort := intstr.FromInt(constants.PortCluster)

	// Build egress rules dynamically based on detected API server information
	egressRules, err := buildDNSEgressRules(cluster)
	if err != nil {
		return nil, err
	}

	// Add service network CIDR rule if detected (works for all cluster types)
	if apiServerInfo != nil && apiServerInfo.ServiceNetworkCIDR != "" {
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			// Allow egress to Kubernetes API server via service network (port 443).
			// This works for managed clusters (EKS, GKE, AKS) where the API server
			// is external and accessed via the service IP.
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

	// Add endpoint IP rules if detected (works for self-managed clusters)
	if apiServerInfo != nil && len(apiServerInfo.EndpointIPs) > 0 {
		for _, endpointIP := range apiServerInfo.EndpointIPs {
			endpointCIDR, err := ipToSingleHostCIDR(endpointIP)
			if err != nil {
				return nil, err
			}
			egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
				// Allow egress to Kubernetes API server endpoint IPs (port 6443).
				// This works for self-managed clusters (k3d, kubeadm) where the API server
				// runs on control plane nodes with specific IPs.
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

	// SECURITY: We no longer use permissive fallback rules. API server detection
	// must succeed before building the NetworkPolicy. This is enforced in ensureNetworkPolicy.
	// If we reach here without API server info, it's a programming error.
	if apiServerInfo == nil || (apiServerInfo.ServiceNetworkCIDR == "" && len(apiServerInfo.EndpointIPs) == 0) {
		return nil, fmt.Errorf("API server information is required but not provided")
	}

	// Add cluster pod communication rule
	egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
		// Allow egress to cluster pods for Raft communication
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

	// Merge user-provided egress rules (append after operator-managed rules)
	if cluster.Spec.Network != nil && len(cluster.Spec.Network.EgressRules) > 0 {
		egressRules = append(egressRules, cluster.Spec.Network.EgressRules...)
	}

	// Build operator-managed ingress rules
	ingressRules := buildNetworkPolicyIngressRules(cluster, clusterPeer, operatorPeer, apiPort, clusterPort)

	// Merge user-provided ingress rules (append after operator-managed rules)
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
				// Exclude backup and restore job pods from this NetworkPolicy.
				// These jobs have different network requirements (e.g., access to object storage)
				// and should be managed by separate NetworkPolicies if restrictions are needed.
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
	egressRules = append(egressRules,
		networkingv1.NetworkPolicyEgressRule{
			// Allow egress to OpenBao API (to fetch/restore snapshots, etc.).
			To: []networkingv1.NetworkPolicyPeer{openBaoPeer},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &openBaoAPIPort,
				},
			},
		},
	)

	// Add service network CIDR rule if detected.
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

	// Add endpoint IP rules if detected (self-managed clusters).
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

	// Development profile convenience: if the user didn't provide explicit egress rules,
	// allow common HTTPS egress for backup/restore targets.
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

	// Respect user-provided egress rules as additional allowances.
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
			// Default deny all ingress to job pods.
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

// ensureGatewayCAConfigMap creates or updates a ConfigMap containing the OpenBaoCluster CA certificate.
// This ConfigMap is required for BackendTLSPolicy when using Traefik Gateway API, as Traefik only
// supports ConfigMap references for CA certificates (not Secrets).
//
