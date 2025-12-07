package infra

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func TestStatefulSetStartsWithOneReplicaWhenNotInitialized(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-init", "default")
	cluster.Status.Initialized = false
	cluster.Spec.Replicas = 3

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	if statefulSet.Spec.Replicas == nil {
		t.Fatalf("expected StatefulSet to have replicas set")
	}

	// Should start with 1 replica when not initialized
	if *statefulSet.Spec.Replicas != 1 {
		t.Fatalf("expected StatefulSet replicas to be 1 when not initialized, got %d", *statefulSet.Spec.Replicas)
	}
}

func TestStatefulSetScalesToDesiredReplicasWhenInitialized(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-scaled", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Replicas = 3

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	if statefulSet.Spec.Replicas == nil {
		t.Fatalf("expected StatefulSet to have replicas set")
	}

	// Should scale to desired replicas when initialized
	if *statefulSet.Spec.Replicas != cluster.Spec.Replicas {
		t.Fatalf("expected StatefulSet replicas to be %d when initialized, got %d", cluster.Spec.Replicas, *statefulSet.Spec.Replicas)
	}
}

func TestStatefulSetReplicaScalingTableDriven(t *testing.T) {
	tests := []struct {
		name         string
		initialized  bool
		specReplicas int32
		wantReplicas int32
	}{
		{
			name:         "not initialized starts with 1 replica",
			initialized:  false,
			specReplicas: 3,
			wantReplicas: 1,
		},
		{
			name:         "initialized scales to desired replicas",
			initialized:  true,
			specReplicas: 3,
			wantReplicas: 3,
		},
		{
			name:         "initialized with 1 replica stays at 1",
			initialized:  true,
			specReplicas: 1,
			wantReplicas: 1,
		},
		{
			name:         "not initialized with 1 replica stays at 1",
			initialized:  false,
			specReplicas: 1,
			wantReplicas: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := newTestClient(t)
			manager := NewManager(k8sClient, testScheme)

			cluster := newMinimalCluster("test-replica", "default")
			cluster.Spec.Replicas = tt.specReplicas
			cluster.Status.Initialized = tt.initialized

			ctx := context.Background()

			if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			sts := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      statefulSetName(cluster),
			}, sts)
			if err != nil {
				t.Fatalf("expected StatefulSet to exist: %v", err)
			}

			if sts.Spec.Replicas == nil {
				t.Fatalf("expected StatefulSet to have replicas set")
			}

			if *sts.Spec.Replicas != tt.wantReplicas {
				t.Fatalf("StatefulSet replicas = %d, want %d", *sts.Spec.Replicas, tt.wantReplicas)
			}
		})
	}
}

func TestStatefulSetHasCorrectContainerConfiguration(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-container", "default")
	cluster.Spec.Config = map[string]string{
		"extra": "ui = true\n",
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	if len(statefulSet.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("expected one VolumeClaimTemplate, got %d", len(statefulSet.Spec.VolumeClaimTemplates))
	}

	containerFound := false
	for _, c := range statefulSet.Spec.Template.Spec.Containers {
		if c.Name == openBaoContainerName {
			containerFound = true
			if c.Image != cluster.Spec.Image {
				t.Fatalf("expected container image %q, got %q", cluster.Spec.Image, c.Image)
			}
			if len(c.VolumeMounts) == 0 {
				t.Fatalf("expected container to have volume mounts")
			}

			// Verify container runs bao server with rendered config path
			if len(c.Command) == 0 {
				t.Fatalf("expected container to have command set")
			}
			if len(c.Command) < 3 || c.Command[0] != openBaoBinaryName || c.Command[1] != "server" {
				t.Fatalf("expected container command to be %s server -config=..., got %v", openBaoBinaryName, c.Command)
			}
			if !strings.Contains(c.Command[2], "-config=") {
				t.Fatalf("expected container command to include -config flag, got %v", c.Command)
			}

			// Verify environment variables are set correctly
			envVars := make(map[string]string)
			for _, env := range c.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.FieldRef != nil {
						envVars[env.Name] = env.ValueFrom.FieldRef.FieldPath
					}
				} else if env.Value != "" {
					envVars[env.Name] = env.Value
				}
			}

			if envVars["HOSTNAME"] != "metadata.name" {
				t.Fatalf("expected HOSTNAME env var to reference metadata.name, got %q", envVars["HOSTNAME"])
			}
			if envVars["POD_IP"] != "status.podIP" {
				t.Fatalf("expected POD_IP env var to reference status.podIP, got %q", envVars["POD_IP"])
			}
			if !strings.Contains(envVars["BAO_API_ADDR"], "$(POD_IP)") {
				t.Fatalf("expected BAO_API_ADDR env var to contain $(POD_IP), got %q", envVars["BAO_API_ADDR"])
			}
			if envVars["UMASK"] != "0077" {
				t.Fatalf("expected UMASK env var to be 0077, got %q", envVars["UMASK"])
			}
		}
	}
	if !containerFound {
		t.Fatalf("expected StatefulSet to have container %q", openBaoContainerName)
	}
}

func TestStatefulSetHasInitContainerWhenEnabled(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-init-container", "default")
	cluster.Spec.InitContainer = &openbaov1alpha1.InitContainerConfig{
		Enabled: true,
		Image:   "openbao/openbao-config-init:latest",
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	if len(statefulSet.Spec.Template.Spec.InitContainers) == 0 {
		t.Fatalf("expected StatefulSet to have init containers when enabled")
	}

	initContainerFound := false
	for _, ic := range statefulSet.Spec.Template.Spec.InitContainers {
		if ic.Name == "bao-config-init" {
			initContainerFound = true
			if ic.Image != "openbao/openbao-config-init:latest" {
				t.Fatalf("expected init container image %q, got %q", "openbao/openbao-config-init:latest", ic.Image)
			}
		}
	}
	if !initContainerFound {
		t.Fatalf("expected StatefulSet to have bao-config-init container")
	}
}

func TestStatefulSetIncludesInitContainerEvenWhenDisabledFlagSet(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-no-init-container", "default")
	cluster.Spec.InitContainer = &openbaov1alpha1.InitContainerConfig{
		Enabled: false,
	}

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	if len(statefulSet.Spec.Template.Spec.InitContainers) == 0 {
		t.Fatalf("expected StatefulSet to always include init containers even when InitContainer.Enabled is false")
	}
}

func TestStatefulSetHasCorrectVolumeMounts(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-volumes", "default")

	ctx := context.Background()

	if err := manager.Reconcile(ctx, logr.Discard(), cluster, ""); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	statefulSet := &appsv1.StatefulSet{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	expectedVolumes := []string{
		tlsVolumeName,
		configVolumeName,
		configRenderedVolumeName,
		unsealVolumeName,
		dataVolumeName,
	}

	containerFound := false
	for _, c := range statefulSet.Spec.Template.Spec.Containers {
		if c.Name == openBaoContainerName {
			containerFound = true
			volumeMountNames := make(map[string]bool)
			for _, vm := range c.VolumeMounts {
				volumeMountNames[vm.Name] = true
			}

			for _, expectedVol := range expectedVolumes {
				if !volumeMountNames[expectedVol] {
					t.Fatalf("expected container to have volume mount %q", expectedVol)
				}
			}
		}
	}
	if !containerFound {
		t.Fatalf("expected StatefulSet to have container %q", openBaoContainerName)
	}
}

func TestDeletePVCsDeletesAllPVCs(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme)

	cluster := newMinimalCluster("infra-delete-pvcs", "default")

	ctx := context.Background()

	// Create PVCs
	pvc1 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-infra-delete-pvcs-0",
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				labelOpenBaoCluster: cluster.Name,
			},
		},
	}
	pvc2 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-infra-delete-pvcs-1",
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				labelOpenBaoCluster: cluster.Name,
			},
		},
	}

	if err := k8sClient.Create(ctx, pvc1); err != nil {
		t.Fatalf("failed to create PVC1: %v", err)
	}
	if err := k8sClient.Create(ctx, pvc2); err != nil {
		t.Fatalf("failed to create PVC2: %v", err)
	}

	// Cleanup with DeletePVCs policy
	if err := manager.Cleanup(ctx, logr.Discard(), cluster, openbaov1alpha1.DeletionPolicyDeletePVCs); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify PVCs are deleted
	err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pvc1.Name,
	}, &corev1.PersistentVolumeClaim{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected PVC1 to be deleted, got error: %v", err)
	}

	err = k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      pvc2.Name,
	}, &corev1.PersistentVolumeClaim{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected PVC2 to be deleted, got error: %v", err)
	}
}
