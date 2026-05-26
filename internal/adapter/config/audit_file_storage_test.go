package config

import (
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRenderHCLWithAuditFileStorageAllowsPathUnderMount(t *testing.T) {
	cluster := newMinimalCluster("audit-storage", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
		Size: "5Gi",
	}
	cluster.Spec.Audit = []openbaov1alpha1.AuditDevice{{
		Type: auditTypeFile,
		Path: "file",
		FileOptions: &openbaov1alpha1.FileAuditOptions{
			FilePath: "/openbao/audit/audit.jsonl",
		},
	}}

	if _, err := RenderHCL(cluster, testInfrastructureDetails(cluster)); err != nil {
		t.Fatalf("RenderHCL() error = %v", err)
	}
}

func TestRenderHCLWithAuditFileStorageRejectsPathOutsideMount(t *testing.T) {
	tests := []struct {
		name     string
		device   openbaov1alpha1.AuditDevice
		wantText string
	}{
		{
			name: "structured file options",
			device: openbaov1alpha1.AuditDevice{
				Type: auditTypeFile,
				Path: "file",
				FileOptions: &openbaov1alpha1.FileAuditOptions{
					FilePath: "/tmp/audit.jsonl",
				},
			},
			wantText: `file audit path "/tmp/audit.jsonl" must be under auditFileStorage.mountPath "/openbao/audit"`,
		},
		{
			name: "raw options",
			device: openbaov1alpha1.AuditDevice{
				Type:    auditTypeFile,
				Path:    "file",
				Options: &apiextensionsv1.JSON{Raw: []byte(`{"file_path":"/bao/data/audit.jsonl"}`)},
			},
			wantText: `file audit path "/bao/data/audit.jsonl" must be under auditFileStorage.mountPath "/openbao/audit"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalCluster("audit-storage", "default")
			cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
				Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
				Size: "5Gi",
			}
			cluster.Spec.Audit = []openbaov1alpha1.AuditDevice{tt.device}

			_, err := RenderHCL(cluster, testInfrastructureDetails(cluster))
			if err == nil {
				t.Fatal("RenderHCL() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("RenderHCL() error = %q, want containing %q", err.Error(), tt.wantText)
			}
		})
	}
}

func TestRenderHCLWithAuditFileStorageRejectsForbiddenMountPath(t *testing.T) {
	cluster := newMinimalCluster("audit-storage", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode:      openbaov1alpha1.AuditFileStorageModeManagedPVC,
		Size:      "5Gi",
		MountPath: "/bao/data/audit",
	}

	_, err := RenderHCL(cluster, testInfrastructureDetails(cluster))
	if err == nil {
		t.Fatal("RenderHCL() expected error, got nil")
	}
	if !strings.Contains(err.Error(), `auditFileStorage.mountPath "/bao/data/audit" must not be /tmp`) {
		t.Fatalf("RenderHCL() error = %q, want forbidden mount path error", err.Error())
	}
}

func TestRenderSelfInitHCLWithAuditFileStorageRejectsPathOutsideMount(t *testing.T) {
	cluster := newMinimalCluster("audit-storage", "default")
	cluster.Spec.AuditFileStorage = &openbaov1alpha1.AuditFileStorageConfig{
		Mode: openbaov1alpha1.AuditFileStorageModeManagedPVC,
		Size: "5Gi",
	}
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{{
			Name:      "enable-file-audit",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/audit/file",
			AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{
				Type: auditTypeFile,
				FileOptions: &openbaov1alpha1.FileAuditOptions{
					FilePath: "/tmp/audit.jsonl",
				},
			},
		}},
	}

	_, err := RenderSelfInitHCL(cluster, nil)
	if err == nil {
		t.Fatal("RenderSelfInitHCL() expected error, got nil")
	}
	wantText := `self-init request 0 "enable-file-audit": file audit path "/tmp/audit.jsonl" must be under auditFileStorage.mountPath "/openbao/audit"`
	if !strings.Contains(err.Error(), wantText) {
		t.Fatalf("RenderSelfInitHCL() error = %q, want containing %q", err.Error(), wantText)
	}
}

func testInfrastructureDetails(cluster *openbaov1alpha1.OpenBaoCluster) InfrastructureDetails {
	return InfrastructureDetails{
		HeadlessServiceName: cluster.Name,
		Namespace:           cluster.Namespace,
		APIPort:             8200,
		ClusterPort:         8201,
	}
}
