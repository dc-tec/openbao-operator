package infra

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const inferredBlueImage = "openbao/openbao:2.4.4"

func TestBlueGreenActiveRevision_SwitchesInCleanup(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Image:    "openbao/openbao:2.5.0",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision:  "blue-rev",
				GreenRevision: "green-rev",
				Phase:         openbaov1alpha1.PhaseCleanup,
			},
		},
	}

	if got := BlueGreenActiveRevision(cluster); got != "green-rev" {
		t.Fatalf("expected green revision in cleanup phase, got %q", got)
	}

	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseDemotingBlue
	if got := BlueGreenActiveRevision(cluster); got != "blue-rev" {
		t.Fatalf("expected blue revision before cleanup phase, got %q", got)
	}
}

func TestEnsureBlueGreenStatus_PrefersCurrentVersionImageDuringUpgrade(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Image:    "openbao/openbao:2.5.0",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(cluster).
		Build()

	EnsureBlueGreenStatus(context.Background(), logr.Discard(), c, cluster)

	if cluster.Status.BlueGreen == nil {
		t.Fatalf("expected BlueGreen status to be initialized")
	}
	wantBlueImage := constants.GetOpenBaoImage("2.4.4")
	if got := cluster.Status.BlueGreen.BlueImage; got != wantBlueImage {
		t.Fatalf("expected BlueImage %q, got %q", wantBlueImage, got)
	}
}

func TestEnsureBlueGreenStatus_UsesPodInference(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = openbaov1alpha1.AddToScheme(scheme)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Image:    "openbao/openbao:2.5.0",
			Replicas: 3,
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			},
		},
	}

	activePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-blue-0",
			Namespace: "default",
			Labels: map[string]string{
				constants.LabelAppName:         constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:     "test",
				constants.LabelAppManagedBy:    constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelOpenBaoCluster:  "test",
				constants.LabelOpenBaoRevision: "blue-rev",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "openbao", Image: inferredBlueImage},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(cluster, activePod).
		Build()

	EnsureBlueGreenStatus(context.Background(), logr.Discard(), c, cluster)

	if cluster.Status.BlueGreen == nil {
		t.Fatalf("expected BlueGreen status to be initialized")
	}
	if got := cluster.Status.BlueGreen.BlueRevision; got != "blue-rev" {
		t.Fatalf("expected BlueRevision blue-rev, got %q", got)
	}
	if got := cluster.Status.BlueGreen.BlueImage; got != inferredBlueImage {
		t.Fatalf("expected BlueImage %s, got %q", inferredBlueImage, got)
	}
}
