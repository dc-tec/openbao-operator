package workload

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestStatefulSet_ACMEMode_NoSidecar(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.Replicas = 1
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	containers := statefulSet.Spec.Template.Spec.Containers
	for _, container := range containers {
		if container.Name == "tls-reloader" {
			t.Fatal("expected StatefulSet to NOT have tls-reloader sidecar in ACME mode")
		}
	}
	if len(containers) != 1 {
		t.Fatalf("expected StatefulSet to have 1 container in ACME mode, got %d", len(containers))
	}
	if containers[0].Name != constants.ContainerBao {
		t.Fatalf("expected container name to be %q, got %q", constants.ContainerBao, containers[0].Name)
	}
}

func TestStatefulSet_ACMEMode_NoTLSVolume(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.Replicas = 1
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	if hasVolume(statefulSet.Spec.Template.Spec.Volumes, tlsVolumeName) {
		t.Fatal("expected StatefulSet to NOT have TLS volume in ACME mode")
	}
	if hasVolumeMount(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, tlsVolumeName) {
		t.Fatal("expected OpenBao container to NOT mount TLS volume in ACME mode")
	}
}

func TestStatefulSet_ACMEMode_WithSharedCacheMount(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
		SharedCache: &openbaov1alpha1.ACMESharedCacheConfig{
			Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
			Size: "1Gi",
		},
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	if !hasVolume(statefulSet.Spec.Template.Spec.Volumes, acmeCacheVolumeName) {
		t.Fatal("expected StatefulSet to include ACME shared cache volume")
	}
	if !hasVolumeMountWithPath(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, acmeCacheVolumeName, "/bao/acme-cache") {
		t.Fatal("expected OpenBao container to mount ACME shared cache volume at /bao/acme-cache")
	}
}

func TestStatefulSet_ACMEMode_NoShareProcessNamespace(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.Replicas = 1
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	shareProcessNamespace := statefulSet.Spec.Template.Spec.ShareProcessNamespace
	if shareProcessNamespace == nil || *shareProcessNamespace {
		t.Fatal("expected ShareProcessNamespace to be false (restored container isolation)")
	}
}

func TestStatefulSet_NonACMEMode_UsesWrapper(t *testing.T) {
	cluster := newMinimalCluster("external-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	containers := statefulSet.Spec.Template.Spec.Containers
	for _, container := range containers {
		if container.Name == "tls-reloader" {
			t.Fatal("expected StatefulSet to NOT have tls-reloader sidecar (wrapper approach)")
		}
	}
	if len(containers) != 1 {
		t.Fatalf("expected StatefulSet to have 1 container, got %d", len(containers))
	}

	openBaoContainer := containers[0]
	if len(openBaoContainer.Command) == 0 || openBaoContainer.Command[0] != "/utils/bao-wrapper" {
		t.Fatalf("expected OpenBao container to use wrapper as entrypoint, got command: %v", openBaoContainer.Command)
	}

	shareProcessNamespace := statefulSet.Spec.Template.Spec.ShareProcessNamespace
	if shareProcessNamespace == nil || *shareProcessNamespace {
		t.Fatal("expected ShareProcessNamespace to be false (restored container isolation)")
	}
}
