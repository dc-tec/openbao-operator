//go:build e2e
// +build e2e

package framework

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// ParseAPIServerEndpointIPs parses a comma-separated list of Kubernetes API server endpoint IPs.
func ParseAPIServerEndpointIPs(raw string) ([]string, error) {
	return normalizeAPIServerEndpointIPs(strings.Split(raw, ","))
}

// APIServerEndpointIPsFromEndpointSlices returns Kubernetes API server endpoint IPs from EndpointSlices.
func APIServerEndpointIPsFromEndpointSlices(endpointSlices []discoveryv1.EndpointSlice) ([]string, error) {
	addresses := make([]string, 0)
	for i := range endpointSlices {
		for _, endpoint := range endpointSlices[i].Endpoints {
			addresses = append(addresses, endpoint.Addresses...)
		}
	}

	return normalizeAPIServerEndpointIPs(addresses)
}

func normalizeAPIServerEndpointIPs(rawIPs []string) ([]string, error) {
	unique := make(map[string]struct{}, len(rawIPs))
	for _, rawIP := range rawIPs {
		rawIP = strings.TrimSpace(rawIP)
		if rawIP == "" {
			continue
		}

		address, err := netip.ParseAddr(rawIP)
		if err != nil {
			return nil, fmt.Errorf("invalid Kubernetes API server endpoint IP %q: %w", rawIP, err)
		}
		if address.Zone() != "" {
			return nil, fmt.Errorf("invalid Kubernetes API server endpoint IP %q: zones are not supported", rawIP)
		}
		unique[address.String()] = struct{}{}
	}

	if len(unique) == 0 {
		return nil, fmt.Errorf("no Kubernetes API server endpoint IPs found")
	}

	endpointIPs := make([]string, 0, len(unique))
	for endpointIP := range unique {
		endpointIPs = append(endpointIPs, endpointIP)
	}
	sort.Strings(endpointIPs)
	return endpointIPs, nil
}
