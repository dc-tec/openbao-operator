package v1alpha1

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOpenBaoClusterDefaulterAddsFinalizer(t *testing.T) {
	defaulter := &openBaoClusterDefaulter{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-finalizer",
			Namespace: "default",
		},
	}

	if err := defaulter.Default(ctx, cluster); err != nil {
		t.Fatalf("Default() error = %v, want no error", err)
	}

	found := false
	for _, f := range cluster.Finalizers {
		if f == OpenBaoClusterFinalizer {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Default() did not add expected finalizer %q, got %v", OpenBaoClusterFinalizer, cluster.Finalizers)
	}
}

func TestOpenBaoClusterDefaulterDoesNotAddFinalizerDuringDeletion(t *testing.T) {
	defaulter := &openBaoClusterDefaulter{}
	ctx := context.Background()

	now := metav1.Now()
	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-deleting",
			Namespace:         "default",
			DeletionTimestamp: &now,
		},
	}

	if err := defaulter.Default(ctx, cluster); err != nil {
		t.Fatalf("Default() error = %v, want no error", err)
	}

	for _, f := range cluster.Finalizers {
		if f == OpenBaoClusterFinalizer {
			t.Fatalf("Default() unexpectedly added finalizer %q during deletion", OpenBaoClusterFinalizer)
		}
	}
}

func TestValidateProfile_DevelopmentAllowsAll(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileDevelopment,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeOperatorManaged, // Development allows OperatorManaged
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			// No SelfInit - Development allows this
			// No Unseal - defaults to static, Development allows this
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err != nil {
		t.Errorf("ValidateCreate() error = %v, want no error", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_HardenedRejectsOperatorManagedTLS(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileHardened,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeOperatorManaged, // Hardened requires External
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			Unseal: &UnsealConfig{
				Type: "awskms", // External KMS is OK
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for Hardened profile with OperatorManaged TLS")
	}
	if err != nil && !strings.Contains(err.Error(), "Hardened profile requires spec.tls.mode to be \"External\"") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'Hardened profile requires spec.tls.mode to be \"External\"'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_HardenedRejectsStaticUnseal(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileHardened,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeExternal,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			// No Unseal config - defaults to static, which is rejected
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for Hardened profile with static unseal")
	}
	if err != nil && !strings.Contains(err.Error(), "Hardened profile requires external KMS unseal") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'Hardened profile requires external KMS unseal'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_HardenedRejectsWithoutSelfInit(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileHardened,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeExternal,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			// No SelfInit - Hardened requires it
			Unseal: &UnsealConfig{
				Type: "awskms",
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for Hardened profile without SelfInit")
	}
	if err != nil && !strings.Contains(err.Error(), "Hardened profile requires spec.selfInit.enabled to be true") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'Hardened profile requires spec.selfInit.enabled to be true'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_HardenedRejectsTlsSkipVerify(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileHardened,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeExternal,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			Unseal: &UnsealConfig{
				Type: "awskms",
				Options: map[string]string{
					"tls_skip_verify": "true", // Hardened rejects this
				},
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for Hardened profile with tls_skip_verify")
	}
	if err != nil && !strings.Contains(err.Error(), "Hardened profile does not allow tls_skip_verify=true") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'Hardened profile does not allow tls_skip_verify=true'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_HardenedAllowsValidConfig(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileHardened,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeExternal,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			Unseal: &UnsealConfig{
				Type: "awskms",
				Options: map[string]string{
					"region": "us-east-1",
				},
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err != nil {
		t.Errorf("ValidateCreate() error = %v, want no error", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_ValidatesOnUpdate(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	oldCluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileDevelopment,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeOperatorManaged,
				RotationPeriod: "720h",
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
		},
	}

	newCluster := oldCluster.DeepCopy()
	newCluster.Spec.Profile = ProfileHardened
	newCluster.Spec.TLS.Mode = TLSModeOperatorManaged // Still OperatorManaged - should fail

	warnings, err := validator.ValidateUpdate(ctx, oldCluster, newCluster)
	if err == nil {
		t.Error("ValidateUpdate() expected error when changing to Hardened profile with invalid config")
	}
	if err != nil && !strings.Contains(err.Error(), "Hardened profile requires spec.tls.mode to be \"External\"") {
		t.Errorf("ValidateUpdate() error = %v, want error containing 'Hardened profile requires spec.tls.mode to be \"External\"'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateUpdate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile(t *testing.T) {
	tests := []struct {
		name    string
		cluster *OpenBaoCluster
		wantErr bool
		errMsg  string
	}{
		{
			name: "Development profile allows all configurations",
			cluster: &OpenBaoCluster{
				Spec: OpenBaoClusterSpec{
					Profile: ProfileDevelopment,
					TLS: TLSConfig{
						Mode: TLSModeOperatorManaged,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Hardened profile requires External TLS",
			cluster: &OpenBaoCluster{
				Spec: OpenBaoClusterSpec{
					Profile: ProfileHardened,
					TLS: TLSConfig{
						Mode: TLSModeOperatorManaged,
					},
					SelfInit: &SelfInitConfig{
						Enabled: true,
					},
					Unseal: &UnsealConfig{
						Type: "awskms",
					},
				},
			},
			wantErr: true,
			errMsg:  "Hardened profile requires spec.tls.mode to be \"External\"",
		},
		{
			name: "Hardened profile requires external KMS unseal",
			cluster: &OpenBaoCluster{
				Spec: OpenBaoClusterSpec{
					Profile: ProfileHardened,
					TLS: TLSConfig{
						Mode: TLSModeExternal,
					},
					SelfInit: &SelfInitConfig{
						Enabled: true,
					},
					// No Unseal - defaults to static
				},
			},
			wantErr: true,
			errMsg:  "Hardened profile requires external KMS unseal",
		},
		{
			name: "Hardened profile requires SelfInit enabled",
			cluster: &OpenBaoCluster{
				Spec: OpenBaoClusterSpec{
					Profile: ProfileHardened,
					TLS: TLSConfig{
						Mode: TLSModeExternal,
					},
					Unseal: &UnsealConfig{
						Type: "awskms",
					},
					// No SelfInit
				},
			},
			wantErr: true,
			errMsg:  "Hardened profile requires spec.selfInit.enabled to be true",
		},
		{
			name: "Hardened profile rejects tls_skip_verify",
			cluster: &OpenBaoCluster{
				Spec: OpenBaoClusterSpec{
					Profile: ProfileHardened,
					TLS: TLSConfig{
						Mode: TLSModeExternal,
					},
					SelfInit: &SelfInitConfig{
						Enabled: true,
					},
					Unseal: &UnsealConfig{
						Type: "awskms",
						Options: map[string]string{
							"tls_skip_verify": "true",
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "Hardened profile does not allow tls_skip_verify=true",
		},
		{
			name: "Hardened profile with valid configuration",
			cluster: &OpenBaoCluster{
				Spec: OpenBaoClusterSpec{
					Profile: ProfileHardened,
					TLS: TLSConfig{
						Mode: TLSModeExternal,
					},
					SelfInit: &SelfInitConfig{
						Enabled: true,
					},
					Unseal: &UnsealConfig{
						Type: "awskms",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateProfile(tt.cluster)
			if tt.wantErr {
				if len(errs) == 0 {
					t.Errorf("validateProfile() expected error but got none")
					return
				}
				if tt.errMsg != "" {
					found := false
					for _, err := range errs {
						if err.Error() != "" && strings.Contains(err.Error(), tt.errMsg) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("validateProfile() error message does not contain %q, got: %v", tt.errMsg, errs)
					}
				}
			} else {
				if len(errs) > 0 {
					t.Errorf("validateProfile() unexpected errors: %v", errs)
				}
			}
		})
	}
}

func TestValidateTLS_ACME_RequiresConfig(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeACME,
				RotationPeriod: "720h", // Required field, but not used in ACME mode
				// ACME config is missing - should fail
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for ACME mode without ACME config")
	}
	if err != nil && !strings.Contains(err.Error(), "ACME configuration is required when tls.mode is ACME") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'ACME configuration is required when tls.mode is ACME'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateTLS_ACME_RequiresDirectoryURL(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeACME,
				RotationPeriod: "720h", // Required field, but not used in ACME mode
				ACME: &ACMEConfig{
					DirectoryURL: "", // Missing - should fail
					Domain:       "example.com",
				},
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for ACME mode without directoryURL")
	}
	if err != nil && !strings.Contains(err.Error(), "ACME directoryURL is required") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'ACME directoryURL is required'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateTLS_ACME_RequiresDomain(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeACME,
				RotationPeriod: "720h", // Required field, but not used in ACME mode
				ACME: &ACMEConfig{
					DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
					Domain:       "", // Missing - should fail
				},
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err == nil {
		t.Error("ValidateCreate() expected error for ACME mode without domain")
	}
	if err != nil && !strings.Contains(err.Error(), "ACME domain is required") {
		t.Errorf("ValidateCreate() error = %v, want error containing 'ACME domain is required'", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateTLS_ACME_ValidConfig(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeACME,
				RotationPeriod: "720h", // Required field, but not used in ACME mode
				ACME: &ACMEConfig{
					DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
					Domain:       "example.com",
					Email:        "admin@example.com",
				},
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err != nil {
		t.Errorf("ValidateCreate() error = %v, want no error", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}

func TestValidateProfile_HardenedAllowsACME(t *testing.T) {
	validator := &openBaoClusterValidator{}
	ctx := context.Background()

	cluster := &OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  ProfileHardened,
			TLS: TLSConfig{
				Enabled:        true,
				Mode:           TLSModeACME,
				RotationPeriod: "720h", // Required field, but not used in ACME mode
				ACME: &ACMEConfig{
					DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
					Domain:       "example.com",
				},
			},
			Storage: StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &InitContainerConfig{
				Enabled: true,
				Image:   "openbao/openbao-config-init:latest",
			},
			SelfInit: &SelfInitConfig{
				Enabled: true,
			},
			Unseal: &UnsealConfig{
				Type: "awskms",
			},
		},
	}

	warnings, err := validator.ValidateCreate(ctx, cluster)
	if err != nil {
		t.Errorf("ValidateCreate() error = %v, want no error for Hardened profile with ACME", err)
	}
	if len(warnings) > 0 {
		t.Errorf("ValidateCreate() warnings = %v, want empty", warnings)
	}
}
