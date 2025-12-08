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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

	cluster := newMinimalCluster("infra-init", "default")
	cluster.Status.Initialized = false
	cluster.Spec.Replicas = 3

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

	cluster := newMinimalCluster("infra-scaled", "default")
	cluster.Status.Initialized = true
	cluster.Spec.Replicas = 3

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

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
			manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

			cluster := newMinimalCluster("test-replica", "default")
			cluster.Spec.Replicas = tt.specReplicas
			cluster.Status.Initialized = tt.initialized

			// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
			createTLSSecretForTest(t, k8sClient, cluster)

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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

	cluster := newMinimalCluster("infra-container", "default")
	cluster.Spec.Config = map[string]string{
		"extra": "ui = true\n",
	}

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

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

			// Verify container runs via the wrapper and passes bao server with rendered config path.
			if len(c.Command) != 1 || c.Command[0] != "/utils/bao-wrapper" {
				t.Fatalf("expected container command to be /utils/bao-wrapper, got %v", c.Command)
			}

			foundChildCmd := false
			for i := range c.Args {
				if c.Args[i] == "--" && len(c.Args) > i+3 &&
					c.Args[i+1] == openBaoBinaryName &&
					c.Args[i+2] == "server" &&
					strings.Contains(c.Args[i+3], "-config=") {
					foundChildCmd = true
					break
				}
			}
			if !foundChildCmd {
				t.Fatalf("expected wrapper args to include %s server -config=..., got %v", openBaoBinaryName, c.Args)
			}

			foundSATokenMount := false
			for _, mount := range c.VolumeMounts {
				if mount.MountPath == serviceAccountMountPath {
					foundSATokenMount = true
					break
				}
			}
			if !foundSATokenMount {
				t.Fatalf("expected ServiceAccount token mount at %s", serviceAccountMountPath)
			}

			if c.StartupProbe == nil || c.StartupProbe.Exec == nil {
				t.Fatalf("expected container to have startup probe")
			}
			if c.LivenessProbe == nil || c.LivenessProbe.Exec == nil {
				t.Fatalf("expected container to have liveness probe")
			}
			if c.ReadinessProbe == nil || c.ReadinessProbe.Exec == nil {
				t.Fatalf("expected container to have readiness probe")
			}

			if len(c.StartupProbe.Exec.Command) == 0 || c.StartupProbe.Exec.Command[0] != openBaoProbeBinary {
				t.Fatalf("expected startup probe to exec %s, got %v", openBaoProbeBinary, c.StartupProbe.Exec.Command)
			}
			if len(c.LivenessProbe.Exec.Command) == 0 || c.LivenessProbe.Exec.Command[0] != openBaoProbeBinary {
				t.Fatalf("expected liveness probe to exec %s, got %v", openBaoProbeBinary, c.LivenessProbe.Exec.Command)
			}
			if len(c.ReadinessProbe.Exec.Command) == 0 || c.ReadinessProbe.Exec.Command[0] != openBaoProbeBinary {
				t.Fatalf("expected readiness probe to exec %s, got %v", openBaoProbeBinary, c.ReadinessProbe.Exec.Command)
			}

			if c.StartupProbe.TimeoutSeconds != 10 {
				t.Fatalf("expected startup probe timeout to be 10s, got %d", c.StartupProbe.TimeoutSeconds)
			}

			if !strings.Contains(strings.Join(c.StartupProbe.Exec.Command, " "), "-addr="+openBaoProbeAddr) {
				t.Fatalf("expected startup probe to target %s, got %v", openBaoProbeAddr, c.StartupProbe.Exec.Command)
			}
			if !strings.Contains(strings.Join(c.StartupProbe.Exec.Command, " "), "-mode=startup") {
				t.Fatalf("expected startup probe to use startup mode, got %v", c.StartupProbe.Exec.Command)
			}
			if !strings.Contains(strings.Join(c.StartupProbe.Exec.Command, " "), "-timeout="+openBaoStartupProbeTimeout) {
				t.Fatalf("expected startup probe to use timeout %s, got %v", openBaoStartupProbeTimeout, c.StartupProbe.Exec.Command)
			}

			if !strings.Contains(strings.Join(c.LivenessProbe.Exec.Command, " "), "-addr="+openBaoProbeAddr) {
				t.Fatalf("expected liveness probe to target %s, got %v", openBaoProbeAddr, c.LivenessProbe.Exec.Command)
			}
			if !strings.Contains(strings.Join(c.LivenessProbe.Exec.Command, " "), "-timeout="+openBaoProbeTimeout) {
				t.Fatalf("expected liveness probe to use timeout %s, got %v", openBaoProbeTimeout, c.LivenessProbe.Exec.Command)
			}

			if !strings.Contains(strings.Join(c.ReadinessProbe.Exec.Command, " "), "-addr="+openBaoProbeAddr) {
				t.Fatalf("expected readiness probe to target %s, got %v", openBaoProbeAddr, c.ReadinessProbe.Exec.Command)
			}
			if !strings.Contains(strings.Join(c.ReadinessProbe.Exec.Command, " "), "-timeout="+openBaoProbeTimeout) {
				t.Fatalf("expected readiness probe to use timeout %s, got %v", openBaoProbeTimeout, c.ReadinessProbe.Exec.Command)
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
			if envVars["BAO_K8S_POD_NAME"] != "metadata.name" {
				t.Fatalf("expected BAO_K8S_POD_NAME env var to reference metadata.name, got %q", envVars["BAO_K8S_POD_NAME"])
			}
			if envVars["BAO_K8S_NAMESPACE"] != "metadata.namespace" {
				t.Fatalf("expected BAO_K8S_NAMESPACE env var to reference metadata.namespace, got %q", envVars["BAO_K8S_NAMESPACE"])
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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

	cluster := newMinimalCluster("infra-init-container", "default")
	cluster.Spec.InitContainer = &openbaov1alpha1.InitContainerConfig{
		Enabled: true,
		Image:   "openbao/openbao-config-init:latest",
	}

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

	cluster := newMinimalCluster("infra-no-init-container", "default")
	cluster.Spec.InitContainer = &openbaov1alpha1.InitContainerConfig{
		Enabled: false,
	}

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

	cluster := newMinimalCluster("infra-volumes", "default")

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

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
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", "")

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

func TestStatefulSet_ACMEMode_NoSidecar(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}

	// Build StatefulSet directly to avoid NetworkPolicy creation issues in tests
	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	// Verify no TLS reloader sidecar
	containers := statefulSet.Spec.Template.Spec.Containers
	hasReloader := false
	for _, container := range containers {
		if container.Name == "tls-reloader" {
			hasReloader = true
			break
		}
	}
	if hasReloader {
		t.Fatal("expected StatefulSet to NOT have tls-reloader sidecar in ACME mode")
	}

	// Verify only one container (OpenBao container)
	if len(containers) != 1 {
		t.Fatalf("expected StatefulSet to have 1 container in ACME mode, got %d", len(containers))
	}
	if containers[0].Name != "openbao" {
		t.Fatalf("expected container name to be 'openbao', got %q", containers[0].Name)
	}
}

func TestStatefulSet_ACMEMode_NoTLSVolume(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}

	// Build StatefulSet directly to avoid NetworkPolicy creation issues in tests
	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	// Verify no TLS volume
	volumes := statefulSet.Spec.Template.Spec.Volumes
	hasTLSVolume := false
	for _, volume := range volumes {
		if volume.Name == "tls" {
			hasTLSVolume = true
			break
		}
	}
	if hasTLSVolume {
		t.Fatal("expected StatefulSet to NOT have TLS volume in ACME mode")
	}

	// Verify OpenBao container doesn't mount TLS volume
	openBaoContainer := statefulSet.Spec.Template.Spec.Containers[0]
	hasTLSMount := false
	for _, mount := range openBaoContainer.VolumeMounts {
		if mount.Name == "tls" {
			hasTLSMount = true
			break
		}
	}
	if hasTLSMount {
		t.Fatal("expected OpenBao container to NOT mount TLS volume in ACME mode")
	}
}

func TestStatefulSet_ACMEMode_NoShareProcessNamespace(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}

	// Build StatefulSet directly to avoid NetworkPolicy creation issues in tests
	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	// Verify ShareProcessNamespace is false (restored isolation for all modes)
	shareProcessNamespace := statefulSet.Spec.Template.Spec.ShareProcessNamespace
	if shareProcessNamespace == nil || *shareProcessNamespace {
		t.Fatal("expected ShareProcessNamespace to be false (restored container isolation)")
	}
}

func TestStatefulSet_NonACMEMode_UsesWrapper(t *testing.T) {
	cluster := newMinimalCluster("external-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal

	// Build StatefulSet directly to avoid NetworkPolicy creation issues in tests
	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	// Verify no TLS reloader sidecar (wrapper approach eliminates need for sidecar)
	containers := statefulSet.Spec.Template.Spec.Containers
	hasReloader := false
	for _, container := range containers {
		if container.Name == "tls-reloader" {
			hasReloader = true
			break
		}
	}
	if hasReloader {
		t.Fatal("expected StatefulSet to NOT have tls-reloader sidecar (wrapper approach)")
	}

	// Verify only one container (OpenBao container with wrapper)
	if len(containers) != 1 {
		t.Fatalf("expected StatefulSet to have 1 container, got %d", len(containers))
	}

	// Verify OpenBao container uses wrapper as entrypoint
	openBaoContainer := containers[0]
	if len(openBaoContainer.Command) == 0 || openBaoContainer.Command[0] != "/utils/bao-wrapper" {
		t.Fatalf("expected OpenBao container to use wrapper as entrypoint, got command: %v", openBaoContainer.Command)
	}

	// Verify ShareProcessNamespace is false (restored isolation)
	shareProcessNamespace := statefulSet.Spec.Template.Spec.ShareProcessNamespace
	if shareProcessNamespace == nil || *shareProcessNamespace {
		t.Fatal("expected ShareProcessNamespace to be false (restored container isolation)")
	}
}
