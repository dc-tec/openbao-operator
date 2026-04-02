package snapshot

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRequireBackupConfig(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := RequireBackupConfig(cluster, false, "backup config required"); err == nil || !strings.Contains(err.Error(), "backup config required") {
		t.Fatalf("RequireBackupConfig() error = %v, want missing config", err)
	}

	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
	if err := RequireBackupConfig(cluster, true, "backup config required"); err == nil || !strings.Contains(err.Error(), "backup config required") {
		t.Fatalf("RequireBackupConfig() with endpoint requirement error = %v, want missing endpoint", err)
	}

	cluster.Spec.Backup.Target.Endpoint = "https://example.com"
	if err := RequireBackupConfig(cluster, true, "backup config required"); err != nil {
		t.Fatalf("RequireBackupConfig() unexpected error: %v", err)
	}
}

func TestValidateBackupAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cluster   *openbaov1alpha1.OpenBaoCluster
		wantError bool
	}{
		{
			name:      "missing backup config",
			cluster:   &openbaov1alpha1.OpenBaoCluster{},
			wantError: true,
		},
		{
			name: "jwt auth role configured",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{JWTAuthRole: "backup"},
				},
			},
		},
		{
			name: "token secret configured",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{
						TokenSecretRef: &corev1.LocalObjectReference{Name: "backup-token"},
					},
				},
			},
		},
		{
			name: "self init oidc enabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{},
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
					},
				},
			},
		},
		{
			name: "no auth configured",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Backup: &openbaov1alpha1.BackupSchedule{},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBackupAuth(tt.cluster, "backup authentication is required")
			if tt.wantError {
				if err == nil || !strings.Contains(err.Error(), "backup authentication is required") {
					t.Fatalf("ValidateBackupAuth() error = %v, want auth error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateBackupAuth() unexpected error: %v", err)
			}
		})
	}
}

func TestBackupTokenSecretNameAndEnsureExists(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Backup: &openbaov1alpha1.BackupSchedule{
				TokenSecretRef: &corev1.LocalObjectReference{Name: "backup-token"},
			},
		},
	}

	secretName, ok := BackupTokenSecretName(cluster)
	if !ok || secretName != "backup-token" {
		t.Fatalf("BackupTokenSecretName() = %q, %v", secretName, ok)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error: %v", err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-token", Namespace: "default"},
	}).Build()

	if err := EnsureBackupTokenSecretExists(context.Background(), client, "default", "backup-token"); err != nil {
		t.Fatalf("EnsureBackupTokenSecretExists() unexpected error: %v", err)
	}
	if err := EnsureBackupTokenSecretExists(context.Background(), client, "default", "missing-token"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("EnsureBackupTokenSecretExists() error = %v, want not found", err)
	}
}

func TestValidateHardenedNetwork(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
		},
	}

	if err := ValidateHardenedNetwork(cluster, "egress rules required"); err == nil || !strings.Contains(err.Error(), "egress rules required") {
		t.Fatalf("ValidateHardenedNetwork() error = %v, want egress requirement", err)
	}

	cluster.Spec.Network = &openbaov1alpha1.NetworkConfig{
		EgressRules: []networkingv1.NetworkPolicyEgressRule{{}},
	}
	if err := ValidateHardenedNetwork(cluster, "egress rules required"); err != nil {
		t.Fatalf("ValidateHardenedNetwork() unexpected error with explicit rules: %v", err)
	}
}

func TestValidatePreUpgradeSnapshotPrerequisites(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error: %v", err)
	}

	t.Run("requires token secret existence when configured", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Backup: &openbaov1alpha1.BackupSchedule{
					Target:         openbaov1alpha1.BackupTarget{Endpoint: "https://example.com"},
					TokenSecretRef: &corev1.LocalObjectReference{Name: "missing-token"},
				},
			},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).Build()

		err := ValidatePreUpgradeSnapshotPrerequisites(context.Background(), client, cluster, ValidationOptions{
			MissingBackupMessage:  "backup config required",
			RequireEndpoint:       true,
			RequireTokenSecret:    true,
			NetworkErrorMessage:   "egress rules required",
			AuthenticationMessage: "auth required",
		})
		if err == nil || !strings.Contains(err.Error(), "missing-token") {
			t.Fatalf("ValidatePreUpgradeSnapshotPrerequisites() error = %v, want missing secret", err)
		}
	})

	t.Run("allows oidc auth without token secret validation", func(t *testing.T) {
		t.Parallel()

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Backup: &openbaov1alpha1.BackupSchedule{
					Target: openbaov1alpha1.BackupTarget{Endpoint: "https://example.com"},
				},
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
				},
			},
		}

		err := ValidatePreUpgradeSnapshotPrerequisites(context.Background(), nil, cluster, ValidationOptions{
			MissingBackupMessage:  "backup config required",
			RequireEndpoint:       true,
			RequireTokenSecret:    false,
			NetworkErrorMessage:   "egress rules required",
			AuthenticationMessage: "auth required",
		})
		if err != nil {
			t.Fatalf("ValidatePreUpgradeSnapshotPrerequisites() unexpected error: %v", err)
		}
	})
}
