package infra

import (
	"context"
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubernetesServiceNamespace = "default"
	kubernetesServiceName      = "kubernetes"
)

type apiServerDiscovery struct {
	reader client.Reader
}

func newAPIServerDiscovery(r client.Reader) *apiServerDiscovery {
	return &apiServerDiscovery{reader: r}
}

func (d *apiServerDiscovery) DiscoverServiceNetworkCIDR(ctx context.Context) (string, error) {
	if d == nil || d.reader == nil {
		return "", fmt.Errorf("kubernetes reader is required")
	}

	kubernetesSvc := &corev1.Service{}
	if err := d.reader.Get(ctx, types.NamespacedName{
		Namespace: kubernetesServiceNamespace,
		Name:      kubernetesServiceName,
	}, kubernetesSvc); err != nil {
		return "", err
	}

	// Derive service network CIDR from the ClusterIP
	// We treat the kubernetes Service IP as a single-host CIDR so it can be
	// used for least-privilege NetworkPolicy egress allow-listing.
	if kubernetesSvc.Spec.ClusterIP == "" || kubernetesSvc.Spec.ClusterIP == "None" {
		return "", nil
	}

	ip := net.ParseIP(strings.TrimSpace(kubernetesSvc.Spec.ClusterIP))
	if ip == nil {
		return "", nil
	}

	if ip.To4() != nil {
		return ip.String() + "/32", nil
	}
	return ip.String() + "/128", nil
}
