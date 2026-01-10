package infra

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

type endpointSliceListErrorClient struct {
	client.Client
	err error
}

func (c endpointSliceListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*discoveryv1.EndpointSliceList); ok {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

func TestAPIServerDiscovery_DiscoverServiceNetworkCIDR(t *testing.T) {
	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceName,
			Namespace: kubernetesServiceNamespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.43.0.1",
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
	if got != "10.43.0.0/16" {
		t.Fatalf("DiscoverServiceNetworkCIDR() = %q, expected %q", got, "10.43.0.0/16")
	}
}

func TestAPIServerDiscovery_DiscoverEndpointIPs_ReadyOnly(t *testing.T) {
	ready := true
	notReady := false

	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes-slice-1",
			Namespace: kubernetesServiceNamespace,
			Labels: map[string]string{
				"kubernetes.io/service-name": kubernetesServiceName,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"1.2.3.4"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: &ready,
				},
			},
			{
				Addresses: []string{"5.6.7.8"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: &notReady,
				},
			},
			{
				Addresses: []string{"9.9.9.9"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: nil,
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(endpointSlice).
		Build()

	discovery := newAPIServerDiscovery(k8sClient)
	got, err := discovery.DiscoverEndpointIPs(context.Background())
	if err != nil {
		t.Fatalf("DiscoverEndpointIPs() error: %v", err)
	}
	if len(got) != 1 || got[0] != "1.2.3.4" {
		t.Fatalf("DiscoverEndpointIPs() = %v, expected %v", got, []string{"1.2.3.4"})
	}
}

func TestDetectAPIServerInfo_ManualCIDRIsNormalized(t *testing.T) {
	m := &Manager{client: fake.NewClientBuilder().WithScheme(testScheme).Build()}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Network: &openbaov1alpha1.NetworkConfig{
				APIServerCIDR: "10.43.0.1/16",
			},
		},
	}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if info.ServiceNetworkCIDR != "10.43.0.0/16" {
		t.Fatalf("ServiceNetworkCIDR=%q, expected %q", info.ServiceNetworkCIDR, "10.43.0.0/16")
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
	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceName,
			Namespace: kubernetesServiceNamespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.43.0.1",
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(kubernetesService).
		Build()
	k8sClient := endpointSliceListErrorClient{Client: baseClient, err: errors.New("list denied")}

	m := &Manager{client: k8sClient}
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
				APIServerCIDR: "10.43.0.0/16",
			},
		},
	}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if info.ServiceNetworkCIDR != "10.43.0.0/16" {
		t.Fatalf("ServiceNetworkCIDR=%q, expected %q", info.ServiceNetworkCIDR, "10.43.0.0/16")
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

func TestDetectAPIServerInfo_EndpointSliceListErrorFallsBackToServiceCIDR(t *testing.T) {
	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceName,
			Namespace: kubernetesServiceNamespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.43.0.1",
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(kubernetesService).
		Build()
	k8sClient := endpointSliceListErrorClient{Client: baseClient, err: errors.New("list denied")}

	m := &Manager{client: k8sClient}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if info.ServiceNetworkCIDR != "10.43.0.0/16" {
		t.Fatalf("ServiceNetworkCIDR=%q, expected %q", info.ServiceNetworkCIDR, "10.43.0.0/16")
	}
	if len(info.EndpointIPs) != 0 {
		t.Fatalf("EndpointIPs=%v, expected empty", info.EndpointIPs)
	}
}

func TestDetectAPIServerInfo_AutoDetectsReadyEndpointIPs(t *testing.T) {
	ready := true

	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetesServiceName,
			Namespace: kubernetesServiceNamespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.43.0.1",
		},
	}

	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes-slice-1",
			Namespace: kubernetesServiceNamespace,
			Labels: map[string]string{
				"kubernetes.io/service-name": kubernetesServiceName,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"192.168.166.2"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: &ready,
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(kubernetesService, endpointSlice).
		Build()

	m := &Manager{client: k8sClient}
	cluster := &openbaov1alpha1.OpenBaoCluster{}

	info, err := m.detectAPIServerInfo(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("detectAPIServerInfo() error: %v", err)
	}
	if len(info.EndpointIPs) != 1 || info.EndpointIPs[0] != "192.168.166.2" {
		t.Fatalf("EndpointIPs=%v, expected %v", info.EndpointIPs, []string{"192.168.166.2"})
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
