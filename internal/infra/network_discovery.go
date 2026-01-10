package infra

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubernetesServiceNamespace = "default"
	kubernetesServiceName      = "kubernetes"
)

type apiServerDiscovery struct {
	client client.Client
}

func newAPIServerDiscovery(c client.Client) *apiServerDiscovery {
	return &apiServerDiscovery{client: c}
}

func (d *apiServerDiscovery) DiscoverServiceNetworkCIDR(ctx context.Context) (string, error) {
	if d == nil || d.client == nil {
		return "", fmt.Errorf("kubernetes client is required")
	}

	kubernetesSvc := &corev1.Service{}
	if err := d.client.Get(ctx, types.NamespacedName{
		Namespace: kubernetesServiceNamespace,
		Name:      kubernetesServiceName,
	}, kubernetesSvc); err != nil {
		return "", err
	}

	// Derive service network CIDR from the ClusterIP
	// For example, if ClusterIP is 10.43.0.1, the CIDR is 10.43.0.0/16
	if kubernetesSvc.Spec.ClusterIP == "" || kubernetesSvc.Spec.ClusterIP == "None" {
		return "", nil
	}

	parts := strings.Split(kubernetesSvc.Spec.ClusterIP, ".")
	if len(parts) < 2 {
		return "", nil
	}

	return fmt.Sprintf("%s.%s.0.0/16", parts[0], parts[1]), nil
}

func (d *apiServerDiscovery) DiscoverEndpointIPs(ctx context.Context) ([]string, error) {
	if d == nil || d.client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}

	endpointSliceList := &discoveryv1.EndpointSliceList{}
	if err := d.client.List(ctx, endpointSliceList,
		client.InNamespace(kubernetesServiceNamespace),
		client.MatchingLabels(map[string]string{
			"kubernetes.io/service-name": kubernetesServiceName,
		}),
	); err != nil {
		return nil, err
	}

	// Extract endpoint IPs from the endpoint slices
	var endpointIPs []string
	for _, endpointSlice := range endpointSliceList.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			// Only include ready endpoints
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
				for _, address := range endpoint.Addresses {
					if strings.TrimSpace(address) != "" {
						endpointIPs = append(endpointIPs, address)
					}
				}
			}
		}
	}

	return endpointIPs, nil
}
