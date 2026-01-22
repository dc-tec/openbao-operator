// Package storage provides cloud-agnostic object storage interfaces and implementations
// for backup operations in the OpenBao Operator.
//
// This file contains Google Cloud Storage implementation using Go CDK.
package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/gcp"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/dc-tec/openbao-operator/internal/interfaces"
)

const gcsRWScope = "https://www.googleapis.com/auth/devstorage.read_write"

// GCSClientConfig holds configuration for Google Cloud Storage.
type GCSClientConfig struct {
	// Bucket is the GCS bucket name.
	Bucket string
	// Project is the GCP project ID (optional if using ADC).
	Project string
	// Endpoint is an optional custom endpoint (useful for fake-gcs-server).
	Endpoint string
	// CredentialsJSON is the service account key JSON (optional if using ADC).
	CredentialsJSON []byte
	// UseEmulator disables authentication and requires a custom Endpoint.
	UseEmulator bool
	// InsecureSkipVerify allows skipping TLS verification (useful for emulators).
	InsecureSkipVerify bool
	// CACert is an optional PEM-encoded CA certificate for custom TLS verification.
	CACert []byte
	// EnsureExists optionally validates/creates the bucket (requires Project for create).
	EnsureExists bool
}

// OpenGCSBucket opens a GCS bucket using Go CDK.
// It returns a BlobStore interface that provides standardized blob operations.
//
// When an Endpoint is provided (for emulators, private clouds, or GCS-compatible stores),
// option.WithEndpoint() is always used to ensure consistent routing for all operations.
// The UseEmulator flag controls authentication: when true, anonymous access is used;
// otherwise, credentials are required.
func OpenGCSBucket(ctx context.Context, cfg GCSClientConfig) (interfaces.BlobStore, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	if cfg.UseEmulator && cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required when useEmulator is true")
	}

	httpClient, err := buildGCSHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	// Build client options - always use explicit endpoint when provided
	// This ensures consistent routing for all GCS operations (upload, head, download)
	// with custom endpoints like emulators, private clouds, or GCS-compatible stores.
	var clientOpts []option.ClientOption
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(normalizeGCSEndpoint(cfg.Endpoint)))
	}

	if cfg.UseEmulator {
		// Emulator mode: use anonymous authentication
		clientOpts = append(clientOpts, option.WithoutAuthentication())
		bucket, err := gcsblob.OpenBucket(ctx, gcp.NewAnonymousHTTPClient(httpClient.Transport), cfg.Bucket, &gcsblob.Options{
			ClientOptions: clientOpts,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to open GCS bucket (emulator): %w", err)
		}

		wrapped := NewBucket(bucket)

		if cfg.EnsureExists {
			if err := ensureGCSBucket(ctx, cfg, wrapped); err != nil {
				_ = wrapped.Close()
				return nil, err
			}
		}

		return wrapped, nil
	}

	// Production mode: use full authentication
	creds, err := buildGCSCredentials(ctx, cfg.CredentialsJSON)
	if err != nil {
		return nil, err
	}

	// Build OAuth-enabled HTTP client with our TLS transport.
	ts := gcp.CredentialsTokenSource(creds)
	if ts == nil {
		return nil, fmt.Errorf("failed to build GCS token source")
	}
	gcpClient, err := gcp.NewHTTPClient(httpClient.Transport, ts)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated HTTP client: %w", err)
	}

	clientOpts = append(clientOpts, option.WithScopes(gcsRWScope))

	// Open bucket using Go CDK gcsblob driver
	opts := &gcsblob.Options{
		ClientOptions: clientOpts,
	}

	bucket, err := gcsblob.OpenBucket(ctx, gcpClient, cfg.Bucket, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open GCS bucket: %w", err)
	}

	wrapped := NewBucket(bucket)

	if cfg.EnsureExists {
		if err := ensureGCSBucket(ctx, cfg, wrapped); err != nil {
			_ = wrapped.Close()
			return nil, err
		}
	}

	return wrapped, nil
}

func buildGCSCredentials(ctx context.Context, credsJSON []byte) (*google.Credentials, error) {
	if len(credsJSON) > 0 {
		return google.CredentialsFromJSON(ctx, credsJSON, gcsRWScope)
	}
	adc, err := google.FindDefaultCredentials(ctx, gcsRWScope)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCS default credentials: %w", err)
	}
	return adc, nil
}

func buildGCSHTTPClient(cfg GCSClientConfig) (*http.Client, error) {
	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
	}

	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		certPool = x509.NewCertPool()
	}

	if len(cfg.CACert) > 0 {
		if !certPool.AppendCertsFromPEM(cfg.CACert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
	}

	tlsConfig := &tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: cfg.InsecureSkipVerify, // #nosec G402 -- Intentional for emulator support
		MinVersion:         tls.VersionTLS12,
	}
	transport.TLSClientConfig = tlsConfig

	return &http.Client{
		Transport: transport,
		Timeout:   DefaultUploadTimeout,
	}, nil
}

func ensureGCSBucket(ctx context.Context, cfg GCSClientConfig, b *Bucket) error {
	var sc *storage.Client
	if !b.bucket.As(&sc) || sc == nil {
		return fmt.Errorf("failed to access underlying GCS client")
	}

	_, err := sc.Bucket(cfg.Bucket).Attrs(ctx)
	if err == nil {
		return nil
	}

	if !errors.Is(err, storage.ErrBucketNotExist) {
		return fmt.Errorf("failed to check GCS bucket %q: %w", cfg.Bucket, err)
	}

	if cfg.Project == "" {
		return fmt.Errorf("gcs bucket %q does not exist and project is not set for creation: %w", cfg.Bucket, err)
	}

	// Attempt to create the bucket if it does not exist.
	if err := sc.Bucket(cfg.Bucket).Create(ctx, cfg.Project, nil); err != nil {
		return fmt.Errorf("failed to create GCS bucket %q: %w", cfg.Bucket, err)
	}
	return nil
}

func normalizeGCSEndpoint(endpoint string) string {
	if endpoint == "" {
		return endpoint
	}
	if !strings.HasSuffix(endpoint, "/") {
		endpoint += "/"
	}
	// Ensure we hit the storage API path expected by google client.
	if !strings.HasSuffix(endpoint, "/storage/v1/") && !strings.Contains(endpoint, "/storage/v1/") {
		endpoint = strings.TrimSuffix(endpoint, "/") + "/storage/v1/"
	}
	return endpoint
}
