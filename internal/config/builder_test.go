package config

import (
	"encoding/json"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

func newMinimalCluster(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &openbaov1alpha1.InitContainerConfig{
				Image: "openbao/openbao-config-init:latest",
			},
		},
	}
}

func TestRenderHCLIncludesCoreStanzas(t *testing.T) {
	cluster := newMinimalCluster("config-hcl", "security")
	cluster.Spec.Config = map[string]string{
		"telemetry": `telemetry { disable_hostname = true }`,
	}

	infraDetails := InfrastructureDetails{
		HeadlessServiceName: cluster.Name,
		Namespace:           cluster.Namespace,
		APIPort:             8200,
		ClusterPort:         8201,
	}

	got, err := RenderHCL(cluster, infraDetails)
	if err != nil {
		t.Fatalf("RenderHCL() error = %v", err)
	}

	content := string(got)
	expected := []string{
		`listener "tcp"`,
		`seal "static"`,
		`storage "raft"`,
		`node_id = "$${HOSTNAME}"`,
		`https://$${HOSTNAME}.config-hcl.security.svc:8200`,
		`https://$${HOSTNAME}.config-hcl.security.svc:8201`,
		`cluster_name     = "config-hcl"`,
	}

	for _, snippet := range expected {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered config.hcl to contain %q, got:\n%s", snippet, content)
		}
	}
}

func TestRenderHCLIncludesLeaderTLSServerNameForAutoJoin(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")

	infraDetails := InfrastructureDetails{
		HeadlessServiceName: cluster.Name,
		Namespace:           cluster.Namespace,
		APIPort:             8200,
		ClusterPort:         8201,
	}

	// Test auto_join configuration
	got, err := RenderHCL(cluster, infraDetails)
	if err != nil {
		t.Fatalf("RenderHCL() error = %v", err)
	}

	content := string(got)

	// Verify that auto_join is present
	if !strings.Contains(content, `auto_join`) {
		t.Fatalf("expected rendered config.hcl to contain auto_join, got:\n%s", content)
	}

	// Verify that leader_tls_servername is present with the correct value
	// HCL formatting may vary, so check for both the key and value separately
	if !strings.Contains(content, `leader_tls_servername`) {
		t.Fatalf("expected rendered config.hcl to contain leader_tls_servername, got:\n%s", content)
	}
	if !strings.Contains(content, `openbao-cluster-test-cluster.local`) {
		t.Fatalf("expected rendered config.hcl to contain openbao-cluster-test-cluster.local, got:\n%s", content)
	}
}

func TestRenderHCLWithSelfInitRequests(t *testing.T) {
	cluster := newMinimalCluster("selfinit-cluster", "default")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "enable-stdout-audit",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/audit/stdout",
				Data: mustJSON(t, map[string]interface{}{
					"type": "file",
					"options": map[string]interface{}{
						"file_path": "/dev/stdout",
						"log_raw":   true,
					},
				}),
			},
		},
	}

	got, err := RenderSelfInitHCL(cluster)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	content := string(got)

	expectedSnippets := []string{
		`initialize "enable-stdout-audit" {`,
		`request "enable-stdout-audit-request" {`,
		`operation = "update"`,
		`path      = "sys/audit/stdout"`,
		`data = {`,
		`options = {`,
		`file_path = "/dev/stdout"`,
		`log_raw   = true`,
		`type = "file"`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered self-init config.hcl to contain %q, got:\n%s", snippet, content)
		}
	}
}

func mustJSON(t *testing.T, v interface{}) *apiextensionsv1.JSON {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	return &apiextensionsv1.JSON{Raw: raw}
}
