package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openbao/operator/internal/constants"
)

func TestExecutorConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ExecutorConfig
		wantErr bool
	}{
		{
			name: "valid JWT auth config",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodJWT,
				JWTAuthRole:      "test-role",
				JWTToken:         "fake-jwt-token",
			},
			wantErr: false,
		},
		{
			name: "valid token auth config",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodToken,
				OpenBaoToken:     "fake-token",
			},
			wantErr: false,
		},
		{
			name: "missing cluster namespace",
			config: &ExecutorConfig{
				ClusterName:     "test-cluster",
				ClusterReplicas: 3,
				BackupEndpoint:  "https://s3.example.com",
				BackupBucket:    "backups",
				TLSCACert:       []byte("fake-ca-cert"),
				AuthMethod:      constants.BackupAuthMethodToken,
				OpenBaoToken:    "fake-token",
			},
			wantErr: true,
		},
		{
			name: "missing cluster name",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodToken,
				OpenBaoToken:     "fake-token",
			},
			wantErr: true,
		},
		{
			name: "invalid replicas",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  0,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodToken,
				OpenBaoToken:     "fake-token",
			},
			wantErr: true,
		},
		{
			name: "missing backup endpoint",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodToken,
				OpenBaoToken:     "fake-token",
			},
			wantErr: true,
		},
		{
			name: "missing backup bucket",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodToken,
				OpenBaoToken:     "fake-token",
			},
			wantErr: true,
		},
		{
			name: "missing TLS CA cert",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				AuthMethod:       constants.BackupAuthMethodToken,
				OpenBaoToken:     "fake-token",
			},
			wantErr: true,
		},
		{
			name: "JWT auth missing role",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodJWT,
				JWTToken:         "fake-jwt-token",
			},
			wantErr: true,
		},
		{
			name: "JWT auth missing token",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodJWT,
				JWTAuthRole:      "test-role",
			},
			wantErr: true,
		},
		{
			name: "token auth missing token",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       constants.BackupAuthMethodToken,
			},
			wantErr: true,
		},
		{
			name: "invalid auth method",
			config: &ExecutorConfig{
				ClusterNamespace: "test-ns",
				ClusterName:      "test-cluster",
				ClusterReplicas:  3,
				BackupEndpoint:   "https://s3.example.com",
				BackupBucket:     "backups",
				TLSCACert:        []byte("fake-ca-cert"),
				AuthMethod:       "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadStorageCredentials(t *testing.T) {
	// Create a temporary directory for test credentials
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func() string
		wantErr bool
	}{
		{
			name: "credentials directory does not exist",
			setup: func() string {
				return filepath.Join(tmpDir, "nonexistent")
			},
			wantErr: false, // Returns nil, not an error
		},
		{
			name: "valid credentials",
			setup: func() string {
				credsDir := filepath.Join(tmpDir, "creds")
				_ = os.MkdirAll(credsDir, 0755)
				_ = os.WriteFile(filepath.Join(credsDir, "accessKeyId"), []byte("test-access-key"), 0600)
				_ = os.WriteFile(filepath.Join(credsDir, "secretAccessKey"), []byte("test-secret-key"), 0600)
				return credsDir
			},
			wantErr: false,
		},
		{
			name: "missing secret access key",
			setup: func() string {
				credsDir := filepath.Join(tmpDir, "creds-incomplete")
				_ = os.MkdirAll(credsDir, 0755)
				_ = os.WriteFile(filepath.Join(credsDir, "accessKeyId"), []byte("test-access-key"), 0600)
				return credsDir
			},
			wantErr: true,
		},
		{
			name: "path traversal attempt",
			setup: func() string {
				return "../../etc/passwd"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credsPath := tt.setup()
			creds, err := loadStorageCredentials(credsPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadStorageCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && creds != nil {
				if creds.AccessKeyID != "" && creds.SecretAccessKey == "" {
					t.Error("loadStorageCredentials() should require both access key and secret key")
				}
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		filePath string
		wantErr  bool
	}{
		{
			name:     "valid path",
			baseDir:  "/tmp",
			filePath: "/tmp/file.txt",
			wantErr:  false,
		},
		{
			name:     "path traversal attempt",
			baseDir:  "/tmp",
			filePath: "/tmp/../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "path outside base",
			baseDir:  "/tmp",
			filePath: "/etc/passwd",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePath(tt.baseDir, tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
