package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
		LogLevel: "debug",
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

	compareGolden(t, "render_hcl_core_stanzas", got)
}

func TestRenderHCLWithStructuredConfiguration(t *testing.T) {
	cluster := newMinimalCluster("structured-config", "default")
	uiEnabled := true
	autoDownload := true
	autoRegister := false
	rotateBytes := int64(10485760) // 10MB
	rotateMaxFiles := int32(7)
	fileUID := int64(1000)

	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
		UI:       &uiEnabled,
		LogLevel: "info",
		Logging: &openbaov1alpha1.LoggingConfig{
			Format:         "json",
			File:           "/var/log/openbao/openbao.log",
			RotateDuration: "24h",
			RotateBytes:    &rotateBytes,
			RotateMaxFiles: &rotateMaxFiles,
			PIDFile:        "/var/run/openbao/openbao.pid",
		},
		Plugin: &openbaov1alpha1.PluginConfig{
			FileUID:          &fileUID,
			FilePermissions:  "0755",
			AutoDownload:     &autoDownload,
			AutoRegister:     &autoRegister,
			DownloadBehavior: "direct",
		},
		DefaultLeaseTTL: "720h",
		MaxLeaseTTL:     "8760h",
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

	compareGolden(t, "render_hcl_structured_config", got)
}

func TestRenderHCLWithAllConfigurationOptions(t *testing.T) {
	cluster := newMinimalCluster("full-config", "default")
	uiEnabled := true
	cacheSize := int64(134217728) // 128MB
	disableCache := false
	detectDeadlocks := true
	rawStorageEndpoint := false
	introspectionEndpoint := true
	disableStandbyReads := false
	impreciseLeaseRoleTracking := true
	unsafeAllowAPIAuditCreation := false
	allowAuditLogPrefixing := true
	enableResponseHeaderHostname := true
	enableResponseHeaderRaftNodeID := false

	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
		UI:                             &uiEnabled,
		LogLevel:                       "warn",
		CacheSize:                      &cacheSize,
		DisableCache:                   &disableCache,
		DetectDeadlocks:                &detectDeadlocks,
		RawStorageEndpoint:             &rawStorageEndpoint,
		IntrospectionEndpoint:          &introspectionEndpoint,
		DisableStandbyReads:            &disableStandbyReads,
		ImpreciseLeaseRoleTracking:     &impreciseLeaseRoleTracking,
		UnsafeAllowAPIAuditCreation:    &unsafeAllowAPIAuditCreation,
		AllowAuditLogPrefixing:         &allowAuditLogPrefixing,
		EnableResponseHeaderHostname:   &enableResponseHeaderHostname,
		EnableResponseHeaderRaftNodeID: &enableResponseHeaderRaftNodeID,
		DefaultLeaseTTL:                "3600h",
		MaxLeaseTTL:                    "7200h",
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

	compareGolden(t, "render_hcl_all_config_options", got)
}

func TestRenderHCLIncludesLeaderTLSServerNameForAutoJoin(t *testing.T) {
	cluster := newMinimalCluster("test-cluster", "default")

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

	compareGolden(t, "render_hcl_auto_join", got)
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
				AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{
					Type: "file",
					FileOptions: &openbaov1alpha1.FileAuditOptions{
						FilePath: "stdout",
					},
				},
			},
		},
	}

	got, err := RenderSelfInitHCL(cluster, nil)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	compareGolden(t, "render_self_init_requests", got)
}

func mustJSON(t *testing.T, v interface{}) *apiextensionsv1.JSON {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	return &apiextensionsv1.JSON{Raw: raw}
}

// goldenFile reads the golden file for the given test name.
// If UPDATE_GOLDEN is set to "true", it writes the provided content to the golden file instead.
func goldenFile(t *testing.T, name string, got []byte) []byte {
	t.Helper()

	goldenPath := filepath.Join("testdata", name+".hcl")

	// If UPDATE_GOLDEN is set, write the current output to the golden file
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		if err := os.WriteFile(goldenPath, got, 0644); err != nil {
			t.Fatalf("failed to write golden file %q: %v", goldenPath, err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return got
	}

	// Read the expected golden file
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %q: %v. Set UPDATE_GOLDEN=true to generate it.", goldenPath, err)
	}

	return want
}

// compareGolden compares the generated HCL with the golden file and reports differences.
func compareGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	want := goldenFile(t, name, got)

	gotStr := string(got)
	wantStr := string(want)

	if gotStr != wantStr {
		t.Errorf("HCL output does not match golden file %q", name)
		t.Errorf("To update the golden file, run: UPDATE_GOLDEN=true go test ./internal/config -run %s", t.Name())

		// Show a simple line-by-line diff for better readability
		gotLines := strings.Split(gotStr, "\n")
		wantLines := strings.Split(wantStr, "\n")

		maxLines := len(gotLines)
		if len(wantLines) > maxLines {
			maxLines = len(wantLines)
		}

		diffCount := 0
		for i := 0; i < maxLines && diffCount < 10; i++ {
			var gotLine, wantLine string
			if i < len(gotLines) {
				gotLine = gotLines[i]
			}
			if i < len(wantLines) {
				wantLine = wantLines[i]
			}

			if gotLine != wantLine {
				diffCount++
				t.Errorf("Line %d:\n  Got:  %q\n  Want: %q", i+1, gotLine, wantLine)
			}
		}

		if diffCount >= 10 {
			t.Errorf("... (showing first 10 differences, %d total lines differ)", maxLines)
		}

		// Also show full output for small diffs (less than 50 lines)
		if maxLines < 50 {
			t.Errorf("\n--- Full Got output:\n%s\n--- Full Want output:\n%s", gotStr, wantStr)
		}
	}
}

func TestRenderOperatorBootstrapHCL(t *testing.T) {
	config := OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		JWTKeysPEM:    []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"},
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderOperatorBootstrapHCL(config)
	if err != nil {
		t.Fatalf("RenderOperatorBootstrapHCL() error = %v", err)
	}

	compareGolden(t, "render_operator_bootstrap", got)
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
				AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{
					Type: "file",
					FileOptions: &openbaov1alpha1.FileAuditOptions{
						FilePath: "stdout",
					},
				},
			},
		},
	}

	bootstrapConfig := &OperatorBootstrapConfig{
		OIDCIssuerURL: "https://kubernetes.default.svc",
		JWTKeysPEM:    []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"},
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	compareGolden(t, "render_self_init_with_bootstrap", got)
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
		JWTKeysPEM:    []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"},
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	compareGolden(t, "render_self_init_backup_upgrade_policies", got)
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
		JWTKeysPEM:    []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"},
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	compareGolden(t, "render_self_init_no_backup_upgrade", got)
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
		JWTKeysPEM:    []string{"-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----\n"},
		OperatorNS:    "openbao-operator-system",
		OperatorSA:    "openbao-operator-controller",
	}

	got, err := RenderSelfInitHCL(cluster, bootstrapConfig)
	if err != nil {
		t.Fatalf("RenderSelfInitHCL() error = %v", err)
	}

	compareGolden(t, "render_self_init_token_secret_ref", got)
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

	compareGolden(t, "render_hcl_acme_mode", got)
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

	compareGolden(t, "render_hcl_acme_mode_no_email", got)
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

func TestRenderHCL_StaticSeal_Default(t *testing.T) {
	cluster := newMinimalCluster("static-seal", "default")
	// No Unseal config - should default to static with operator defaults

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

	compareGolden(t, "render_hcl_static_seal_default", got)
}

func TestRenderHCL_StaticSeal_Custom(t *testing.T) {
	cluster := newMinimalCluster("static-seal-custom", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "static",
		Static: &openbaov1alpha1.StaticSealConfig{
			CurrentKey:   "file:///custom/path/key",
			CurrentKeyID: "custom-key-v1",
		},
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

	compareGolden(t, "render_hcl_static_seal_custom", got)
}

func TestRenderHCL_TransitSeal(t *testing.T) {
	cluster := newMinimalCluster("transit-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "transit",
		Transit: &openbaov1alpha1.TransitSealConfig{
			Address:       "https://openbao:8200",
			KeyName:       "transit-key",
			MountPath:     "transit/",
			Namespace:     "ns1/",
			TLSCACert:     "/etc/openbao/ca_cert.pem",
			TLSSkipVerify: boolPtr(false),
		},
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

	compareGolden(t, "render_hcl_transit_seal", got)
}

func TestRenderHCL_AWSKMSSeal(t *testing.T) {
	cluster := newMinimalCluster("awskms-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "awskms",
		AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
			Region:   "us-east-1",
			KMSKeyID: "alias/my-key",
			Endpoint: "https://kms.us-east-1.amazonaws.com",
		},
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

	compareGolden(t, "render_hcl_awskms_seal", got)
}

func TestRenderHCL_AzureKeyVaultSeal(t *testing.T) {
	cluster := newMinimalCluster("azure-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "azurekeyvault",
		AzureKeyVault: &openbaov1alpha1.AzureKeyVaultSealConfig{
			VaultName: "my-vault",
			KeyName:   "my-key",
			TenantID:  "tenant-123",
			ClientID:  "client-456",
			Resource:  "managedhsm.azure.net",
		},
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

	compareGolden(t, "render_hcl_azure_keyvault_seal", got)
}

func TestRenderHCL_GCPCloudKMSSeal(t *testing.T) {
	cluster := newMinimalCluster("gcp-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "gcpckms",
		GCPCloudKMS: &openbaov1alpha1.GCPCloudKMSSealConfig{
			Project:     "my-project",
			Region:      "us-central1",
			KeyRing:     "my-keyring",
			CryptoKey:   "my-cryptokey",
			Credentials: "/etc/gcp/credentials.json",
		},
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

	compareGolden(t, "render_hcl_gcp_cloudkms_seal", got)
}

func TestRenderHCL_KMIPSeal(t *testing.T) {
	cluster := newMinimalCluster("kmip-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "kmip",
		KMIP: &openbaov1alpha1.KMIPSealConfig{
			Address:       "kmip.example.com:5696",
			Certificate:   "/etc/kmip/cert.pem",
			Key:           "/etc/kmip/key.pem",
			CACert:        "/etc/kmip/ca.pem",
			TLSServerName: "kmip.example.com",
			TLSSkipVerify: boolPtr(false),
		},
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

	compareGolden(t, "render_hcl_kmip_seal", got)
}

func TestRenderHCL_OCIKMSSeal(t *testing.T) {
	cluster := newMinimalCluster("oci-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "ocikms",
		OCIKMS: &openbaov1alpha1.OCIKMSSealConfig{
			KeyID:              "ocid1.key.oc1..example",
			CryptoEndpoint:     "https://kms.us-ashburn-1.oraclecloud.com",
			ManagementEndpoint: "https://kms.us-ashburn-1.oraclecloud.com",
			AuthType:           "instance_principal",
			CompartmentID:      "ocid1.compartment.oc1..example",
		},
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

	compareGolden(t, "render_hcl_ocikms_seal", got)
}

func TestRenderHCL_PKCS11Seal(t *testing.T) {
	cluster := newMinimalCluster("pkcs11-seal", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "pkcs11",
		PKCS11: &openbaov1alpha1.PKCS11SealConfig{
			Lib:          "/usr/lib/libpkcs11.so",
			Slot:         "0",
			KeyLabel:     "vault-key",
			HMACKeyLabel: "vault-hmac-key",
			GenerateKey:  boolPtr(true),
			RSAOAEPHash:  "sha256",
		},
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

	compareGolden(t, "render_hcl_pkcs11_seal", got)
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}
