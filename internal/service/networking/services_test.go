package networking

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func TestEnsureExternalService_UsesSharedClientSelector(t *testing.T) {
	cluster := newMinimalCluster("svc-test", "default")
	cluster.Spec.Service = &openbaov1alpha1.ServiceConfig{}
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 2}

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(client, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureExternalService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureExternalService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := client.Get(context.Background(), types.NamespacedName{
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

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(client, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureExternalService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureExternalService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := client.Get(context.Background(), types.NamespacedName{
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
				"example.com/expose": "true",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(client, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureReadReplicaService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureReadReplicaService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.ReadReplicaServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get read Service: %v", err)
	}

	if got := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; got != constants.LabelValueOpenBaoWorkloadPoolReadReplica {
		t.Fatalf("selector workload pool = %q, want %q", got, constants.LabelValueOpenBaoWorkloadPoolReadReplica)
	}
	if got := service.Annotations["example.com/expose"]; got != "true" {
		t.Fatalf("annotation = %q, want %q", got, "true")
	}
}

func TestEnsureHeadlessService_RemainsBroadAcrossPools(t *testing.T) {
	cluster := newMinimalCluster("svc-headless", "default")
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 1}

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithReturnManagedFields().
		Build()
	manager := NewManager(client, testScheme, "operators", constants.PlatformKubernetes)

	if err := manager.ensureHeadlessService(context.Background(), logr.Discard(), cluster); err != nil {
		t.Fatalf("ensureHeadlessService() error = %v", err)
	}

	service := &corev1.Service{}
	if err := client.Get(context.Background(), types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      resourceidentity.HeadlessServiceName(cluster),
	}, service); err != nil {
		t.Fatalf("failed to get headless Service: %v", err)
	}

	if _, ok := service.Spec.Selector[constants.LabelOpenBaoWorkloadPool]; ok {
		t.Fatalf("did not expect headless Service selector to pin a workload pool")
	}
}
