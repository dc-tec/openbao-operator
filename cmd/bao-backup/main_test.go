package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

func TestAuthenticate_Token(t *testing.T) {
	cfg := &backupconfig.ExecutorConfig{
		AuthMethod:   constants.BackupAuthMethodToken,
		OpenBaoToken: "test-token",
	}

	token, err := authenticate(context.Background(), cfg, "http://localhost:8200")
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
}

func TestAuthenticate_JWT(t *testing.T) {
	// Mock OpenBao server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/v1/auth/jwt-operator/login", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "test-jwt", body["jwt"])
		assert.Equal(t, "test-role", body["role"])

		// Response
		resp := map[string]interface{}{
			"auth": map[string]interface{}{
				"client_token": "login-token",
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := &backupconfig.ExecutorConfig{
		AuthMethod:  constants.BackupAuthMethodJWT,
		JWTAuthRole: "test-role",
		JWTToken:    "test-jwt",
	}

	token, err := authenticate(context.Background(), cfg, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "login-token", token)
}

func TestParseDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "empty", input: "", want: 0},
		{name: "invalid", input: "nonsense", want: 0},
		{name: "valid", input: "45s", want: 45 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDuration(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExitCodeForError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: exitSuccess},
		{name: "config", err: errors.New("config error"), want: exitConfigError},
		{name: "load config text", err: wrapErr("failed to load configuration"), want: exitConfigError},
		{name: "auth", err: wrapErr("failed to authenticate"), want: exitAuthError},
		{name: "leader", err: wrapErr("failed to find leader"), want: exitLeaderDiscovery},
		{name: "snapshot", err: wrapErr("failed to get snapshot"), want: exitSnapshotError},
		{name: "restore snapshot", err: wrapErr("failed to restore snapshot"), want: exitSnapshotError},
		{name: "upload", err: wrapErr("failed to upload backup"), want: exitStorageError},
		{name: "download", err: wrapErr("failed to download snapshot"), want: exitStorageError},
		{name: "storage client", err: wrapErr("failed to create storage client"), want: exitStorageError},
		{name: "verify", err: wrapErr("failed to verify"), want: exitVerificationError},
		{name: "fallback", err: wrapErr("unexpected"), want: exitConfigError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, exitCodeForError(tt.err))
		})
	}
}

func TestResolveRestoreSettings(t *testing.T) {
	cfg := &backupconfig.ExecutorConfig{
		BackupBucket:   "backup-bucket",
		BackupEndpoint: "https://s3.example.test",
		BackupRegion:   "eu-west-1",
	}

	t.Run("nil config", func(t *testing.T) {
		_, err := resolveRestoreSettings(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "restore configuration is required")
	})

	t.Run("missing key", func(t *testing.T) {
		t.Setenv("RESTORE_KEY", "")
		_, err := resolveRestoreSettings(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "RESTORE_KEY environment variable is required")
	})

	t.Run("uses backup defaults", func(t *testing.T) {
		t.Setenv("RESTORE_KEY", "snap-001")
		t.Setenv("RESTORE_BUCKET", "")
		t.Setenv("RESTORE_ENDPOINT", "")
		t.Setenv("RESTORE_REGION", "")
		t.Setenv("RESTORE_USE_PATH_STYLE", "")

		settings, err := resolveRestoreSettings(cfg)
		require.NoError(t, err)
		assert.Equal(t, "snap-001", settings.key)
		assert.Equal(t, cfg.BackupBucket, settings.bucket)
		assert.Equal(t, cfg.BackupEndpoint, settings.endpoint)
		assert.Equal(t, cfg.BackupRegion, settings.region)
		assert.False(t, settings.usePathStyle)
	})

	t.Run("uses restore overrides", func(t *testing.T) {
		t.Setenv("RESTORE_KEY", "snap-002")
		t.Setenv("RESTORE_BUCKET", "restore-bucket")
		t.Setenv("RESTORE_ENDPOINT", "http://minio:9000")
		t.Setenv("RESTORE_REGION", "us-east-2")
		t.Setenv("RESTORE_USE_PATH_STYLE", "TRUE")

		settings, err := resolveRestoreSettings(cfg)
		require.NoError(t, err)
		assert.Equal(t, "snap-002", settings.key)
		assert.Equal(t, "restore-bucket", settings.bucket)
		assert.Equal(t, "http://minio:9000", settings.endpoint)
		assert.Equal(t, "us-east-2", settings.region)
		assert.True(t, settings.usePathStyle)
	})
}

func TestBuildStorageConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		_, err := buildStorageConfig(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage configuration is required")
	})

	t.Run("s3 default provider and http endpoint infer insecure", func(t *testing.T) {
		cfg := &backupconfig.ExecutorConfig{
			BackupProvider:     "",
			BackupBucket:       "bucket",
			BackupEndpoint:     "http://minio:9000",
			BackupRegion:       "us-east-1",
			BackupUsePathStyle: true,
		}
		got, err := buildStorageConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, storage.ProviderS3, got.Provider)
		assert.Equal(t, "bucket", got.Bucket)
		assert.NotNil(t, got.S3)
		assert.True(t, got.S3.UsePathStyle)
		assert.True(t, got.S3.InsecureSkipVerify)
		assert.NotNil(t, got.Credentials)
		assert.Equal(t, "us-east-1", got.Credentials.Region)
	})

	t.Run("s3 uses provided credentials", func(t *testing.T) {
		cfg := &backupconfig.ExecutorConfig{
			BackupProvider: constants.StorageProviderS3,
			BackupBucket:   "bucket",
			BackupEndpoint: "https://s3.example.test",
			BackupRegion:   "us-west-2",
			StorageCredentials: &blobstore.Credentials{
				AccessKeyID: "akid",
				Region:      "custom-region",
			},
			InsecureSkipVerify: true,
		}
		got, err := buildStorageConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, "custom-region", got.Credentials.Region)
		assert.True(t, got.S3.InsecureSkipVerify)
	})

	t.Run("gcs emulator over http infers insecure", func(t *testing.T) {
		cfg := &backupconfig.ExecutorConfig{
			BackupProvider: constants.StorageProviderGCS,
			BackupBucket:   "gcs-bucket",
			BackupEndpoint: "http://fake-gcs-server:4443",
			GCSUseEmulator: true,
			GCSProject:     "project-a",
		}
		got, err := buildStorageConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, storage.ProviderGCS, got.Provider)
		require.NotNil(t, got.GCS)
		assert.True(t, got.GCS.UseEmulator)
		assert.True(t, got.GCS.InsecureSkipVerify)
		assert.Equal(t, "project-a", got.GCS.Project)
	})

	t.Run("azure container overrides bucket", func(t *testing.T) {
		cfg := &backupconfig.ExecutorConfig{
			BackupProvider:      constants.StorageProviderAzure,
			BackupBucket:        "default-bucket",
			AzureContainer:      "tenant-container",
			AzureStorageAccount: "storage-account",
		}
		got, err := buildStorageConfig(cfg)
		require.NoError(t, err)
		assert.Equal(t, storage.ProviderAzure, got.Provider)
		assert.Equal(t, "tenant-container", got.Bucket)
		require.NotNil(t, got.Azure)
		assert.Equal(t, "storage-account", got.Azure.StorageAccount)
	})

	t.Run("unknown provider", func(t *testing.T) {
		cfg := &backupconfig.ExecutorConfig{
			BackupProvider: "invalid-provider",
			BackupBucket:   "bucket",
		}
		_, err := buildStorageConfig(cfg)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "unknown storage provider"))
	})
}

func wrapErr(msg string) error {
	return errors.New(msg)
}
