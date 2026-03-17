package jobenv

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

func FuzzBackupJWTEnvBuilders(f *testing.F) {
	f.Add(true, "custom-role", true, true, "https://s3.example", "bucket-a", "prefix", "aud")
	f.Add(false, "", true, false, "", "bucket-b", "", "")

	f.Fuzz(func(t *testing.T, oidcEnabled bool, configuredRole string, hasTokenSecret, hasRoleARN bool, endpoint, bucket, pathPrefix, audience string) {
		t.Setenv("OPENBAO_JWT_AUDIENCE", sanitizeJobenvText(audience, ""))

		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-a",
				Namespace: "default",
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Replicas: 3,
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: oidcEnabled},
				},
				Backup: &openbaov1alpha1.BackupSchedule{
					JWTAuthRole: strings.TrimSpace(configuredRole),
					Target: openbaov1alpha1.BackupTarget{
						Provider:   constants.StorageProviderS3,
						Endpoint:   sanitizeJobenvText(endpoint, "https://s3.example"),
						Bucket:     sanitizeJobenvText(bucket, "backups"),
						PathPrefix: sanitizeJobenvText(pathPrefix, ""),
					},
				},
			},
		}
		if hasRoleARN {
			cluster.Spec.Backup.Target.RoleARN = "arn:aws:iam::123456789012:role/test"
		}
		if hasTokenSecret {
			cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: "backup-token"}
		}

		role := EffectiveBackupJWTRole(cluster)
		env := BuildEnvVars(cluster, Options{
			ClientConfig: portopenbao.ClientConfig{
				RateLimitQPS:                   1.5,
				RateLimitBurst:                 4,
				CircuitBreakerFailureThreshold: 3,
			},
		}, "/var/run/secrets/tokens/openbao-token")

		got := make(map[string]string, len(env))
		for _, item := range env {
			got[item.Name] = item.Value
		}

		if role != "" {
			if got[constants.EnvBackupJWTAuthRole] != role {
				t.Fatalf("expected JWT auth role env to match effective role")
			}
			if got[constants.EnvBackupAuthMethod] != constants.BackupAuthMethodJWT {
				t.Fatalf("expected JWT auth method when effective role is set")
			}
		} else if hasTokenSecret {
			if got[constants.EnvBackupAuthMethod] != constants.BackupAuthMethodToken {
				t.Fatalf("expected token auth method when only token secret is configured")
			}
		}

		if audience := OpenBaoJWTAudience(); role == portauth.RoleNameBackup || strings.TrimSpace(configuredRole) != "" || oidcEnabled {
			if strings.TrimSpace(audience) == "" {
				t.Fatalf("expected non-empty OpenBao JWT audience")
			}
		}
	})
}

func sanitizeJobenvText(input, fallback string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(input, "\x00", ""))
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}
