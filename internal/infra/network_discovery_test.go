package infra

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	ClusterIP               = "10.43.0.1"
	KubernetesServiceCIDR   = "10.43.0.1/32"
	ManualAPIServerCIDR     = "10.43.0.1/16"
	NormalizedAPIServerCIDR = "10.43.0.0/16"
)

func TestAPIServerDiscovery_DiscoverServiceNetworkCIDR(t *testing.T) {
	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceName,
			Namespace: kubernetesServiceNamespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: ClusterIP,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(kubernetesService).
		Build()

	discovery := newAPIServerDiscovery(k8sClient)
	got, err := discovery.DiscoverServiceNetworkCIDR(context.Background())
	if err != nil {
		t.Fatalf("DiscoverServiceNetworkCIDR() error: %v", err)
	}
	if got != KubernetesServiceCIDR {
		t.Fatalf("DiscoverServiceNetworkCIDR() = %q, expected %q", got, KubernetesServiceCIDR)
	}
}

func TestDetectAPIServerInfo_ManualCIDRIsNormalized(t *testing.T) {
	m := &Manager{client: fake.NewClientBuilder().WithScheme(testScheme).Build()}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Network: &openbaov1alpha1.NetworkConfig{
				APIServerCIDR: ManualAPIServerCIDR,
			},
		},
	}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if info.ServiceNetworkCIDR != NormalizedAPIServerCIDR {
		t.Fatalf("ServiceNetworkCIDR=%q, expected %q", info.ServiceNetworkCIDR, NormalizedAPIServerCIDR)
	}
}

func TestDetectAPIServerInfo_InvalidManualCIDRErrors(t *testing.T) {
	m := &Manager{client: fake.NewClientBuilder().WithScheme(testScheme).Build()}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Network: &openbaov1alpha1.NetworkConfig{
				APIServerCIDR: "not-a-cidr",
			},
		},
	}

	_, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestDetectAPIServerInfo_ManualEndpointIPsSkipDiscovery(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", ClusterIP)
	m := &Manager{client: fake.NewClientBuilder().WithScheme(testScheme).Build()}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Network: &openbaov1alpha1.NetworkConfig{
				APIServerEndpointIPs: []string{" 192.168.1.2 ", ""},
			},
		},
	}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if len(info.EndpointIPs) != 1 || info.EndpointIPs[0] != "192.168.1.2" {
		t.Fatalf("EndpointIPs=%v, expected %v", info.EndpointIPs, []string{"192.168.1.2"})
	}
}

func TestDetectAPIServerInfo_ServiceGetNotFoundWithManualCIDRSucceeds(t *testing.T) {
	m := &Manager{client: fake.NewClientBuilder().WithScheme(testScheme).Build()}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Network: &openbaov1alpha1.NetworkConfig{
				APIServerCIDR: KubernetesServiceCIDR,
			},
		},
	}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if info.ServiceNetworkCIDR != KubernetesServiceCIDR {
		t.Fatalf("ServiceNetworkCIDR=%q, expected %q", info.ServiceNetworkCIDR, KubernetesServiceCIDR)
	}
}

func TestDetectAPIServerInfo_ServiceGetNotFoundWithoutManualCIDRErrors(t *testing.T) {
	m := &Manager{client: fake.NewClientBuilder().WithScheme(testScheme).Build()}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{},
	}

	_, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !apierrors.IsNotFound(err) {
		// The error is wrapped; verify the root cause is NotFound.
		var statusErr *apierrors.StatusError
		if !errors.As(err, &statusErr) || statusErr.ErrStatus.Reason != metav1.StatusReasonNotFound {
			t.Fatalf("expected a NotFound root cause, got: %v", err)
		}
	}
}

func TestDetectAPIServerInfo_InvalidManualEndpointIPEntryErrors(t *testing.T) {
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	m := &Manager{client: k8sClient}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Network: &openbaov1alpha1.NetworkConfig{
				APIServerEndpointIPs: []string{"not-an-ip"},
			},
		},
	}

	_, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
