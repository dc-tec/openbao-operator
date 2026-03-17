package restore

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func FuzzRestoreJWTEnvAndVolumeSelection(f *testing.F) {
	f.Add(true, "restore-role", true, true, "aud-1")
	f.Add(false, "", true, false, "")
	f.Add(true, "", false, false, "openbao")

	f.Fuzz(func(t *testing.T, oidcEnabled bool, configuredRole string, hasTokenSecret, hasTLS bool, audience string) {
		t.Setenv("OPENBAO_JWT_AUDIENCE", sanitizeRestoreText(audience, ""))

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-a",
				Namespace: "default",
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Replicas: 3,
				TLS: openbaov1alpha1.TLSConfig{
					Enabled: hasTLS,
				},
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
						Enabled: oidcEnabled,
					},
				},
				Backup: &openbaov1alpha1.BackupSchedule{
					Image: "openbao-backup:0.1.0",
				},
			},
		}

		restoreReq := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "restore-a",
				Namespace: "default",
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{
				Cluster: "cluster-a",
				Source: openbaov1alpha1.RestoreSource{
					Key: "default/cluster-a/2025-01-01T00-00-00Z-12345678.snap",
					Target: openbaov1alpha1.BackupTarget{
						Provider: constants.StorageProviderS3,
						Endpoint: "https://s3.example",
						Bucket:   "backups",
					},
				},
				JWTAuthRole: strings.TrimSpace(configuredRole),
			},
		}
		if hasTokenSecret {
			restoreReq.Spec.TokenSecretRef = &corev1.LocalObjectReference{Name: "restore-token"}
		}

		role := effectiveRestoreJWTRole(restoreReq, cluster)
		envVars := buildRestoreEnvVars(restoreReq, cluster)
		got := make(map[string]string, len(envVars))
		for _, item := range envVars {
			got[item.Name] = item.Value
		}
		tlsTrust := portopenbao.TrustBundleSource{UseSystemRoots: !hasTLS}
		volumes := buildRestoreVolumes(restoreReq, cluster, tlsTrust)
		mounts := buildRestoreVolumeMounts(restoreReq, cluster, tlsTrust)

		if role != "" {
			expected := strings.TrimSpace(configuredRole)
			if expected == "" {
				expected = portauth.RoleNameRestore
			}
			if got[constants.EnvBackupJWTAuthRole] != expected {
				t.Fatalf("expected restore JWT role env %q, got %q", expected, got[constants.EnvBackupJWTAuthRole])
			}
			if got[constants.EnvBackupAuthMethod] != constants.BackupAuthMethodJWT {
				t.Fatalf("expected JWT auth method for restore")
			}
			if !containsRestoreVolume(volumes, restoreJWTTokenVolumeName) {
				t.Fatalf("expected JWT token volume for restore JWT auth")
			}
			if !containsRestoreMount(mounts, restoreJWTTokenVolumeName) {
				t.Fatalf("expected JWT token mount for restore JWT auth")
			}
		} else if hasTokenSecret {
			if got[constants.EnvBackupAuthMethod] != constants.BackupAuthMethodToken {
				t.Fatalf("expected token auth method when restore token secret is present")
			}
			if !containsRestoreVolume(volumes, restoreTokenVolumeName) {
				t.Fatalf("expected static token volume when token secret is present")
			}
		}
	})
}

func containsRestoreVolume(volumes []corev1.Volume, name string) bool {
	for _, volume := range volumes {
		if volume.Name == name {
			return true
		}
	}
	return false
}

func containsRestoreMount(mounts []corev1.VolumeMount, name string) bool {
	for _, mount := range mounts {
		if mount.Name == name {
			return true
		}
	}
	return false
}

func sanitizeRestoreText(input, fallback string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(input, "\x00", ""))
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}
