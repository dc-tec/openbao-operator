package networking

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestEnsureExternalService_UsesSharedClientSelector(t *testing.T) {
	cluster := newMinimalCluster("svc-test", "default")
	cluster.Spec.Service = &openbaov1alpha1.ServiceConfig{}
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureExternalService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureExternalService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      externalServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get external Service: %v", err)
	}

	if _, ok := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; ok {
		t.Fatalf("did not expect external Service selector to pin a workload pool")
	}
}

func TestEnsureExternalService_BlueGreenSelectorStillPinsActiveRevision(t *testing.T) {
	cluster := newMinimalCluster("svc-bluegreen", "default")
	cluster.Spec.Service = &openbaov1alpha1.ServiceConfig{}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
	}
	cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
		Phase:        openbaov1alpha1.PhasePromoting,
		BlueRevision: "blue123",
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureExternalService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureExternalService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      externalServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get external Service: %v", err)
	}

	if got := service.Spec.Selector[constants.LabelOpenBaoRevision]; got != "blue123" {
		t.Fatalf("selector revision = %q, want %q", got, "blue123")
	}
	if _, ok := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; ok {
		t.Fatalf("did not expect external Service selector to pin a workload pool during blue/green")
	}
}

func TestEnsureReadReplicaService_CreatesDedicatedSelector(t *testing.T) {
	cluster := newMinimalCluster("svc-read", "default")
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{
		Replicas: 2,
		Service: &openbaov1alpha1.ReadReplicaServiceConfig{
			Enabled: true,
			Annotations: map[string]string{
				"example.com/expose": labelValueTrue,
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureReadReplicaService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureReadReplicaService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ReadReplicaServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get read Service: %v", err)
	}

	if got := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; got != constants.LabelValueOpenBaoWorkloadPoolReadReplica {
		t.Fatalf("selector workload pool = %q, want %q", got, constants.LabelValueOpenBaoWorkloadPoolReadReplica)
	}
	if got := service.Annotations["example.com/expose"]; got != labelValueTrue {
		t.Fatalf("annotation = %q, want %q", got, labelValueTrue)
	}
}

func TestEnsureMetricsService_CreatesActiveMetricsSelector(t *testing.T) {
	cluster := newMinimalCluster("svc-metrics", "default")
	cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
		Metrics: &openbaov1alpha1.MetricsConfig{Enabled: true},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureMetricsService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureMetricsService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      metricsServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get metrics Service: %v", err)
	}

	if got := service.Spec.Selector[portopenbao.LabelActive]; got != labelValueTrue {
		t.Fatalf("selector %s = %q, want true", portopenbao.LabelActive, got)
	}
	if _, ok := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; ok {
		t.Fatalf("did not expect metrics Service selector to pin a workload pool")
	}
	if got := service.Labels[metricsScrapeProfileLabel]; got != metricsScrapeProfileActive {
		t.Fatalf("scrape profile label = %q, want %q", got, metricsScrapeProfileActive)
	}
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Name != metricsServicePortName {
		t.Fatalf("unexpected metrics Service ports: %#v", service.Spec.Ports)
	}
}

func TestEnsureMetricsService_AllNodesUsesHeadlessMetricsListener(t *testing.T) {
	cluster := newMinimalCluster("svc-all-nodes", "default")
	cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
		Metrics: &openbaov1alpha1.MetricsConfig{
			Enabled:       true,
			ScrapeProfile: metricsScrapeProfileAllNode,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureMetricsService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureMetricsService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      metricsServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get metrics Service: %v", err)
	}

	if _, ok := service.Spec.Selector[portopenbao.LabelActive]; ok {
		t.Fatalf("did not expect all-node metrics Service selector to require active pod")
	}
	if service.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Fatalf("clusterIP = %q, want headless", service.Spec.ClusterIP)
	}
	if !service.Spec.PublishNotReadyAddresses {
		t.Fatalf("expected all-node metrics Service to publish not-ready addresses")
	}
	if got := service.Labels[metricsScrapeProfileLabel]; got != metricsScrapeProfileAllNode {
		t.Fatalf("scrape profile label = %q, want %q", got, metricsScrapeProfileAllNode)
	}
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != constants.PortMetrics {
		t.Fatalf("unexpected metrics Service ports: %#v", service.Spec.Ports)
	}
}

func TestEnsureHeadlessService_RemainsBroadAcrossPools(t *testing.T) {
	cluster := newMinimalCluster("svc-headless", "default")
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 1}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureHeadlessService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureHeadlessService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.HeadlessServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get headless Service: %v", err)
	}

	if _, ok := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; ok {
		t.Fatalf("did not expect headless Service selector to pin a workload pool")
	}
}

func TestDeleteServiceIfExistsRejectsUnownedService(t *testing.T) {
	cluster := newMinimalCluster("svc-delete", "default")
	cluster.UID = types.UID("svc-delete-uid")
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalServiceName(cluster),
			Namespace: cluster.Namespace,
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster, service).
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	err := manager.deleteServiceIfExists(context.Background(), cluster, service.Name)
	if err == nil {
		t.Fatal("deleteServiceIfExists() error = nil, want owner proof error")
	}

	current := &corev1.Service{}
	if getErr := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(service), current); getErr != nil {
		t.Fatalf("expected unowned Service to remain: %v", getErr)
	}
}

func TestDeleteServiceIfExistsDeletesOwnedService(t *testing.T) {
	cluster := newMinimalCluster("svc-delete-owned", "default")
	cluster.UID = types.UID("svc-delete-owned-uid")
	controller := true
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      externalServiceName(cluster),
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: openbaov1alpha1.GroupVersion.String(),
				Kind:       "OpenBaoCluster",
				Name:       cluster.Name,
				UID:        cluster.UID,
				Controller: &controller,
			}},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster, service).
		Build()
	manager := NewManager(k8sClient, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.deleteServiceIfExists(context.Background(), cluster, service.Name); err != nil {
		t.Fatalf("deleteServiceIfExists() error = %v", err)
	}

	current := &corev1.Service{}
	if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(service), current); err == nil {
		t.Fatal("expected owned Service to be deleted")
	}
}
