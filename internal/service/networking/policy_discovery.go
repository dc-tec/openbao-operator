package networking

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

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

	if !manualCIDRConfigured {
		if cidr, ok := kubernetesServiceIPCIDRFromEnv(); ok {
			info.ServiceNetworkCIDR = cidr
			logger.V(1).Info("Using kubernetes Service IP CIDR from environment", "cidr", cidr)
		}
	}

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

	return info, nil
}
