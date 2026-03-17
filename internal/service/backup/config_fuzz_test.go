package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func FuzzLoadBackupAuthConfig(f *testing.F) {
	f.Add("jwt", "backup-role", "jwt-token", "static-token", false)
	f.Add("token", "", "", "static-token", false)
	f.Add("", "backup-role", "jwt-token", "", true)

	f.Fuzz(func(t *testing.T, authMethod, jwtRole, jwtToken, staticToken string, missingJWTFile bool) {
		tmpDir := t.TempDir()
		jwtPath := filepath.Join(tmpDir, "jwt")
		tokenPath := filepath.Join(tmpDir, "token")

		if !missingJWTFile {
			if err := os.WriteFile(jwtPath, []byte(sanitizeBackupConfigText(jwtToken)), 0o600); err != nil {
				t.Fatalf("failed to write JWT token file: %v", err)
			}
		}
		if err := os.WriteFile(tokenPath, []byte(sanitizeBackupConfigText(staticToken)), 0o600); err != nil {
			t.Fatalf("failed to write static token file: %v", err)
		}

		t.Setenv(constants.EnvBackupAuthMethod, sanitizeBackupConfigText(authMethod))
		t.Setenv(constants.EnvBackupJWTAuthRole, sanitizeBackupConfigText(jwtRole))
		t.Setenv(constants.EnvJWTTokenPath, jwtPath)
		t.Setenv(constants.EnvBackupTokenPath, tokenPath)

		cfg := &ExecutorConfig{}
		err := loadAuthConfig(cfg)
		if err == nil {
			switch cfg.AuthMethod {
			case constants.BackupAuthMethodJWT:
				if strings.TrimSpace(cfg.JWTAuthRole) == "" || strings.TrimSpace(cfg.JWTToken) == "" {
					t.Fatalf("JWT auth success requires role and token")
				}
			case constants.BackupAuthMethodToken:
				if strings.TrimSpace(cfg.OpenBaoToken) == "" {
					t.Fatalf("token auth success requires OpenBao token")
				}
			default:
				t.Fatalf("unexpected auth method %q", cfg.AuthMethod)
			}
		}
	})
}

func FuzzBackupExecutorValidateAuthModes(f *testing.F) {
	f.Add("jwt", "role", "jwt", "", true)
	f.Add("token", "", "", "token", true)
	f.Add("invalid", "", "", "", false)

	f.Fuzz(func(t *testing.T, authMethod, jwtRole, jwtToken, staticToken string, withTLS bool) {
		cfg := &ExecutorConfig{
			ClusterNamespace: "default",
			ClusterName:      "cluster",
			ClusterReplicas:  1,
			BackupProvider:   constants.StorageProviderS3,
			BackupEndpoint:   "https://s3.example",
			BackupBucket:     "backups",
			AuthMethod:       sanitizeBackupConfigText(authMethod),
			JWTAuthRole:      sanitizeBackupConfigText(jwtRole),
			JWTToken:         sanitizeBackupConfigText(jwtToken),
			OpenBaoToken:     sanitizeBackupConfigText(staticToken),
		}
		if withTLS {
			cfg.TLSCACert = []byte("ca")
		}

		err := cfg.Validate()
		if err == nil {
			switch cfg.AuthMethod {
			case constants.BackupAuthMethodJWT:
				if cfg.JWTAuthRole == "" || cfg.JWTToken == "" {
					t.Fatalf("JWT validation passed without required fields")
				}
			case constants.BackupAuthMethodToken:
				if cfg.OpenBaoToken == "" {
					t.Fatalf("token validation passed without token")
				}
			}
		}
	})
}

func sanitizeBackupConfigText(input string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(input, "\x00", ""))
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}
