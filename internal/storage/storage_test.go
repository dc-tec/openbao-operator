package storage

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenBlobStore_RequiresBucket(t *testing.T) {
	ctx := context.Background()

	_, err := OpenBlobStore(ctx, Config{
		Provider: ProviderS3,
		Bucket:   "",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")
}

func TestOpenBlobStore_UnknownProvider(t *testing.T) {
	ctx := context.Background()

	_, err := OpenBlobStore(ctx, Config{
		Provider: "unknown",
		Bucket:   "test-bucket",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown storage provider")
}

func TestOpenBlobStore_S3DefaultProvider(t *testing.T) {
	ctx := context.Background()

	// Empty provider should default to S3
	// This will fail because of missing credentials/config, but we're testing the provider routing
	_, err := OpenBlobStore(ctx, Config{
		Provider: "", // Empty should default to S3
		Bucket:   "test-bucket",
	})

	// Should get an S3-specific error (missing region or config), not "unknown provider"
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unknown storage provider")
}

func TestOpenBlobStore_GCSMissingBucket(t *testing.T) {
	ctx := context.Background()

	_, err := OpenBlobStore(ctx, Config{
		Provider: ProviderGCS,
		Bucket:   "",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")
}

func TestOpenBlobStore_AzureMissingConfig(t *testing.T) {
	ctx := context.Background()

	_, err := OpenBlobStore(ctx, Config{
		Provider: ProviderAzure,
		Bucket:   "test-container",
		// Missing StorageAccount and ConnectionString
		Azure: nil,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage account or connection string is required")
}

func TestOpenBlobStore_AzureWithConnectionString(t *testing.T) {
	ctx := context.Background()

	// This will fail to connect, but we're testing the config routing
	_, err := OpenBlobStore(ctx, Config{
		Provider: ProviderAzure,
		Bucket:   "test-container",
		Azure: &AzureOptions{
			ConnectionString: "DefaultEndpointsProtocol=https;AccountName=devstoreaccount1;AccountKey=key123;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1",
		},
	})

	// Should fail on connection, not config validation
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "storage account or connection string is required")
}

func TestProviderTypeConstants(t *testing.T) {
	// Ensure provider constants match CRD enum values
	assert.Equal(t, ProviderType("s3"), ProviderS3)
	assert.Equal(t, ProviderType("gcs"), ProviderGCS)
	assert.Equal(t, ProviderType("azure"), ProviderAzure)
}

func TestOpenGCSBucket_EmulatorEndpointRequired(t *testing.T) {
	ctx := context.Background()

	_, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:      "test-bucket",
		UseEmulator: true,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint is required")
}

func TestOpenGCSBucket_EmulatorEnsureExistsHitsEndpoint(t *testing.T) {
	ctx := context.Background()
	var seen int64

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&seen, 1)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/b/test-bucket":
			_, _ = w.Write([]byte(`{"name":"test-bucket"}`))
			return
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	store, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:             "test-bucket",
		Project:            "proj",
		Endpoint:           server.URL,
		UseEmulator:        true,
		InsecureSkipVerify: true,
		EnsureExists:       true,
	})

	require.NoError(t, err)
	defer func() {
		_ = store.Close()
	}()

	if got := atomic.LoadInt64(&seen); got == 0 {
		t.Fatalf("expected emulator endpoint to be called, got %d requests", got)
	}
}

func TestOpenAzureContainer_ManagedIdentityRequiresAccount(t *testing.T) {
	ctx := context.Background()

	_, err := OpenAzureContainer(ctx, AzureClientConfig{
		Container:          "test",
		UseManagedIdentity: true,
		StorageAccount:     "",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "storage account is required")
}

func TestOpenAzureContainer_ConnectionStringEnsureExists(t *testing.T) {
	ctx := context.Background()
	var created int64

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.RawQuery == "restype=container" {
			atomic.AddInt64(&created, 1)
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer server.Close()

	accountKey := base64.StdEncoding.EncodeToString([]byte("testkey"))
	cfg := AzureClientConfig{
		Container:          "test-container",
		StorageAccount:     "devstoreaccount1",
		ConnectionString:   fmt.Sprintf("DefaultEndpointsProtocol=https;AccountName=%s;AccountKey=%s;BlobEndpoint=%s", "devstoreaccount1", accountKey, server.URL),
		InsecureSkipVerify: true,
		EnsureExists:       true,
	}

	store, err := OpenAzureContainer(ctx, cfg)
	require.NoError(t, err)
	defer func() {
		_ = store.Close()
	}()

	if atomic.LoadInt64(&created) == 0 {
		t.Fatalf("expected container creation call to emulator endpoint")
	}
}
