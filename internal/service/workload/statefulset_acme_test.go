package workload

import (
	"slices"
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

func TestStatefulSet_ProbeDefaults(t *testing.T) {
	cluster := newMinimalCluster("probe-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	openBaoContainer := statefulSet.Spec.Template.Spec.Containers[0]
	if openBaoContainer.StartupProbe == nil || openBaoContainer.StartupProbe.Exec == nil {
		t.Fatal("expected startup probe exec action")
	}
	if !slices.Contains(openBaoContainer.StartupProbe.Exec.Command, "-mode=startup") {
		t.Fatalf("startup probe command=%v, want -mode=startup", openBaoContainer.StartupProbe.Exec.Command)
	}
	if !slices.Contains(openBaoContainer.StartupProbe.Exec.Command, "-ca-file="+constants.PathTLSCACert) {
		t.Fatalf("startup probe command=%v, want TLS CA file", openBaoContainer.StartupProbe.Exec.Command)
	}
	if openBaoContainer.StartupProbe.InitialDelaySeconds != 10 {
		t.Fatalf("startup initial delay=%d, want 10", openBaoContainer.StartupProbe.InitialDelaySeconds)
	}
	if openBaoContainer.ReadinessProbe == nil {
		t.Fatal("expected readiness probe")
	}
	if openBaoContainer.ReadinessProbe.InitialDelaySeconds != 20 {
		t.Fatalf("readiness initial delay=%d, want 20", openBaoContainer.ReadinessProbe.InitialDelaySeconds)
	}
}

func TestStatefulSet_ACMEMode_ProbeTrustUsesACMEPKICA(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
	}
	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
		ACMECARoot: "/etc/bao/seal-creds/ca.crt",
	}

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	openBaoContainer := statefulSet.Spec.Template.Spec.Containers[0]
	startupCommand := openBaoContainer.StartupProbe.Exec.Command
	if !slices.Contains(startupCommand, "-servername=example.com") {
		t.Fatalf("startup probe command=%v, want ACME server name", startupCommand)
	}
	if !slices.Contains(startupCommand, "-ca-file=/etc/bao/seal-creds/pki-ca.crt") {
		t.Fatalf("startup probe command=%v, want ACME PKI CA file", startupCommand)
	}

	readinessCommand := openBaoContainer.ReadinessProbe.Exec.Command
	if !slices.Contains(readinessCommand, "-servername=example.com") {
		t.Fatalf("readiness probe command=%v, want ACME server name", readinessCommand)
	}
	if !slices.Contains(readinessCommand, "-ca-file=/etc/bao/seal-creds/pki-ca.crt") {
		t.Fatalf("readiness probe command=%v, want ACME PKI CA file", readinessCommand)
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

func TestStatefulSet_NonACMEMode_RendersSingleIsolatedContainer(t *testing.T) {
	cluster := newMinimalCluster("external-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal

	statefulSet, err := buildStatefulSet(cluster, "test-config", true, "", "", "")
	if err != nil {
		t.Fatalf("buildStatefulSet() error = %v", err)
	}

	containers := statefulSet.Spec.Template.Spec.Containers
	for _, container := range containers {
		if container.Name == "tls-reloader" {
			t.Fatal("expected StatefulSet to omit tls-reloader sidecar")
		}
	}
	if len(containers) != 1 {
		t.Fatalf("expected StatefulSet to have 1 container, got %d", len(containers))
	}

	openBaoContainer := containers[0]
	const wantEntrypoint = "/utils/bao-wrapper"
	gotEntrypoint := ""
	if len(openBaoContainer.Command) > 0 {
		gotEntrypoint = openBaoContainer.Command[0]
	}
	if gotEntrypoint != wantEntrypoint {
		t.Fatalf("OpenBao container command[0] = %q, want %q", gotEntrypoint, wantEntrypoint)
	}

	shareProcessNamespace := statefulSet.Spec.Template.Spec.ShareProcessNamespace
	if shareProcessNamespace == nil || *shareProcessNamespace {
		t.Fatal("expected ShareProcessNamespace to be false (restored container isolation)")
	}
}
