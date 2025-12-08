package config

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
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
		`address              = "0.0.0.0:8200"`,
		`cluster_address      = "0.0.0.0:8201"`,
		`seal "static"`,
		`storage "raft"`,
		`service_registration "kubernetes"`,
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

	got, err := RenderSelfInitHCL(cluster, nil)
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

func TestRenderOperatorBootstrapHCL(t *testing.T) {
	config := OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		CACertPEM:     "-----BEGIN CERTIFICATE-----\ntest-ca-cert\n-----END CERTIFICATE-----",
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderOperatorBootstrapHCL(config)
	if err != nil {
		t.Fatalf("RenderOperatorBootstrapHCL() error = %v", err)
	}

	content := string(got)

	expectedSnippets := []string{
		`initialize "operator-bootstrap" {`,
		`request "enable-jwt-auth" {`,
		`sys/auth/jwt`,
		`type`,
		`"jwt"`,
		`request "config-jwt-auth" {`,
		`auth/jwt/config`,
		`oidc_discovery_url`,
		`oidc_discovery_ca_pem`,
		`bound_issuer`,
		`request "create-operator-policy" {`,
		`sys/policies/acl/openbao-operator`,
		`sys/health`,
		`sys/step-down`,
		`sys/storage/raft/snapshot`,
		`request "create-operator-role" {`,
		`auth/jwt/role/openbao-operator`,
		`role_type`,
		`bound_audiences`,
		`openbao-internal`,
		`bound_claims`,
		`kubernetes.io/namespace`,
		`openbao-operator-system`,
		`kubernetes.io/serviceaccount/name`,
		`openbao-operator-controller`,
		`token_policies`,
		`openbao-operator`,
		`ttl`,
		`"1h"`,
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered bootstrap HCL to contain %q, got:\n%s", snippet, content)
		}
	}
}

func TestRenderSelfInitHCL_WithBootstrapConfig(t *testing.T) {
	cluster := newMinimalCluster("hardened-cluster", "default")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "enable-stdout-audit",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/audit/stdout",
				Data: mustJSON(t, map[string]interface{}{
					"type": "file",
				}),
			},
		},
	}

	bootstrapConfig := &OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		CACertPEM:     "test-ca-cert",
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	content := string(got)

	// Verify bootstrap blocks are present
	bootstrapSnippets := []string{
		`initialize "operator-bootstrap" {`,
		`request "enable-jwt-auth" {`,
		`request "config-jwt-auth" {`,
		`request "create-operator-policy" {`,
		`request "create-operator-role" {`,
	}

	for _, snippet := range bootstrapSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered self-init HCL with bootstrap to contain %q, got:\n%s", snippet, content)
		}
	}

	// Verify user requests are also present
	if !strings.Contains(content, `initialize "enable-stdout-audit"`) {
		t.Fatalf("expected rendered self-init HCL to contain user request, got:\n%s", content)
	}
}

func TestRenderSelfInitHCL_AutoCreatesBackupAndUpgradePolicies(t *testing.T) {
	cluster := newMinimalCluster("hardened-cluster", "default")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
	}
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule:    "0 3 * * *",
		JWTAuthRole: "backup",
		Target: openbaov1alpha1.BackupTarget{
			Endpoint: "https://s3.amazonaws.com",
			Bucket:   "backups",
		},
	}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		JWTAuthRole: "upgrade",
	}

	bootstrapConfig := &OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		CACertPEM:     "test-ca-cert",
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	content := string(got)

	// Verify backup policy and role are automatically created
	backupSnippets := []string{
		`request "create-backup-policy" {`,
		`sys/policies/acl/backup`,
		`sys/storage/raft/snapshot`,
		`request "create-backup-jwt-role" {`,
		`auth/jwt/role/backup`,
		`hardened-cluster-backup-serviceaccount`,
		`"backup"`,
	}

	for _, snippet := range backupSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered self-init HCL to contain backup bootstrap %q, got:\n%s", snippet, content)
		}
	}

	// Verify upgrade policy and role are automatically created
	upgradeSnippets := []string{
		`request "create-upgrade-policy" {`,
		`sys/policies/acl/upgrade`,
		`sys/health`,
		`sys/step-down`,
		`sys/storage/raft/snapshot`,
		`request "create-upgrade-jwt-role" {`,
		`auth/jwt/role/upgrade`,
		`hardened-cluster-upgrade-serviceaccount`,
		`"upgrade"`,
	}

	for _, snippet := range upgradeSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered self-init HCL to contain upgrade bootstrap %q, got:\n%s", snippet, content)
		}
	}
}

func TestRenderSelfInitHCL_DoesNotCreateBackupUpgradePoliciesWhenNotConfigured(t *testing.T) {
	cluster := newMinimalCluster("hardened-cluster", "default")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
	}
	// Backup and upgrade are not configured with jwtAuthRole

	bootstrapConfig := &OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		CACertPEM:     "test-ca-cert",
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	content := string(got)

	// Verify backup and upgrade policies/roles are NOT created when not configured
	shouldNotContain := []string{
		`request "create-backup-policy"`,
		`request "create-backup-jwt-role"`,
		`request "create-upgrade-policy"`,
		`request "create-upgrade-jwt-role"`,
	}

	for _, snippet := range shouldNotContain {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected rendered self-init HCL to NOT contain %q when backup/upgrade not configured, got:\n%s", snippet, content)
		}
	}
}

func TestRenderSelfInitHCL_DoesNotCreatePoliciesWhenUsingTokenSecretRef(t *testing.T) {
	cluster := newMinimalCluster("hardened-cluster", "default")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
	}
	// Backup and upgrade are configured but use TokenSecretRef instead of JWTAuthRole
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule: "0 3 * * *",
		Target: openbaov1alpha1.BackupTarget{
			Endpoint: "https://s3.amazonaws.com",
			Bucket:   "backups",
		},
		TokenSecretRef: &corev1.SecretReference{
			Name:      "backup-token",
			Namespace: "default",
		},
		// JWTAuthRole is not set - using TokenSecretRef instead
	}
	cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
		TokenSecretRef: &corev1.SecretReference{
			Name:      "upgrade-token",
			Namespace: "default",
		},
		// JWTAuthRole is not set - using TokenSecretRef instead
	}

	bootstrapConfig := &OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		CACertPEM:     "test-ca-cert",
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	content := string(got)

	// Verify backup and upgrade policies/roles are NOT created when using TokenSecretRef
	// (opt-in behavior: only creates when JWTAuthRole is explicitly set)
	shouldNotContain := []string{
		`request "create-backup-policy"`,
		`request "create-backup-jwt-role"`,
		`request "create-upgrade-policy"`,
		`request "create-upgrade-jwt-role"`,
	}

	for _, snippet := range shouldNotContain {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected rendered self-init HCL to NOT contain %q when using TokenSecretRef (not JWTAuthRole), got:\n%s", snippet, content)
		}
	}
}

func TestRenderHCL_ACMEMode_RendersACMEConfig(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
		Email:        "admin@example.com",
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

	// Verify ACME parameters are present
	expected := []string{
		`tls_acme_ca_directory`,
		`"https://acme-v02.api.letsencrypt.org/directory"`,
		`tls_acme_domains`,
		`"example.com"`,
		`tls_acme_email`,
		`"admin@example.com"`,
	}

	for _, snippet := range expected {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected rendered config.hcl to contain %q, got:\n%s", snippet, content)
		}
	}

	// Verify file-based TLS paths are NOT present
	shouldNotContain := []string{
		`tls_cert_file`,
		`tls_key_file`,
		`tls_client_ca_file`,
		`/etc/bao/tls/`,
	}

	for _, snippet := range shouldNotContain {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected rendered config.hcl to NOT contain %q (ACME mode), got:\n%s", snippet, content)
		}
	}
}

func TestRenderHCL_ACMEMode_NoRetryJoinTLSFiles(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		Domain:       "example.com",
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

	// Verify retry_join does NOT have TLS file paths in ACME mode
	shouldNotContain := []string{
		`leader_ca_cert_file`,
		`leader_client_cert_file`,
		`leader_client_key_file`,
	}

	for _, snippet := range shouldNotContain {
		if strings.Contains(content, snippet) {
			t.Fatalf("expected rendered config.hcl to NOT contain %q in retry_join (ACME mode), got:\n%s", snippet, content)
		}
	}

	// Verify retry_join still has auto_join and leader_tls_servername
	if !strings.Contains(content, `auto_join`) {
		t.Fatalf("expected rendered config.hcl to contain auto_join, got:\n%s", content)
	}
	if !strings.Contains(content, `leader_tls_servername`) {
		t.Fatalf("expected rendered config.hcl to contain leader_tls_servername, got:\n%s", content)
	}
}

func TestRenderHCL_ACMEMode_RequiresACMEConfig(t *testing.T) {
	cluster := newMinimalCluster("acme-cluster", "default")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	// ACME config is missing

	infraDetails := InfrastructureDetails{
		HeadlessServiceName: cluster.Name,
		Namespace:           cluster.Namespace,
		APIPort:             8200,
		ClusterPort:         8201,
	}

	_, err := RenderHCL(cluster, infraDetails)
	if err == nil {
		t.Fatal("RenderHCL() expected error for ACME mode without ACME config")
	}
	if !strings.Contains(err.Error(), "ACME configuration is required when tls.mode is ACME") {
		t.Fatalf("RenderHCL() error = %v, want error containing 'ACME configuration is required when tls.mode is ACME'", err)
	}
}
