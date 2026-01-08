// Package storage provides cloud-agnostic object storage interfaces and implementations
// for backup operations in the OpenBao Operator.
//
// This file contains AWS S3 / S3-compatible storage implementation using Go CDK.
// For Azure Blob Storage, see azure.go (when available).
// For Google Cloud Storage, see gcs.go (when available).
package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"gocloud.dev/blob"
	"gocloud.dev/blob/s3blob"
	"gocloud.dev/gcerrors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

const (
	// DefaultUploadTimeout is the default timeout for upload operations.
	DefaultUploadTimeout = 30 * time.Minute
)

// SecretKey constants define the expected keys in credentials Secrets.
const (
	// SecretKeyAccessKeyID is the key for the access key ID.
	SecretKeyAccessKeyID = "accessKeyId"
	// SecretKeySecretAccessKey is the key for the secret access key.
	SecretKeySecretAccessKey = "secretAccessKey"
	// SecretKeySessionToken is the optional key for session tokens.
	SecretKeySessionToken = "sessionToken"
	// SecretKeyRegion is the optional key for region override.
	SecretKeyRegion = "region"
	// SecretKeyCACert is the optional key for a custom CA certificate.
	SecretKeyCACert = "caCert"
)

// ObjectInfo contains metadata about an object in storage.
type ObjectInfo struct {
	// Key is the full object key/path in the bucket.
	Key string
	// Size is the object size in bytes.
	Size int64
	// LastModified is when the object was last modified.
	LastModified time.Time
	// ETag is the entity tag for the object (typically an MD5 hash).
	ETag string
}

// Bucket wraps a Go CDK blob.Bucket with a simplified interface.
// It implements the common operations needed for backup/restore functionality.
type Bucket struct {
	bucket *blob.Bucket
}

// NewBucket creates a Bucket wrapper around a Go CDK blob.Bucket.
func NewBucket(bucket *blob.Bucket) *Bucket {
	return &Bucket{bucket: bucket}
}

// Upload stores the contents of body as an object with the given key.
// For large objects, Go CDK automatically handles multipart uploads.
func (b *Bucket) Upload(ctx context.Context, key string, body io.Reader) error {
	w, err := b.bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(w, body)
	closeErr := w.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// Delete removes the object with the given key.
// Returns nil if the object does not exist.
func (b *Bucket) Delete(ctx context.Context, key string) error {
	err := b.bucket.Delete(ctx, key)
	if err != nil && gcerrors.Code(err) == gcerrors.NotFound {
		return nil
	}
	return err
}

// DeleteBatch removes multiple objects at once.
// This is a convenience method that calls Delete for each key.
func (b *Bucket) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := b.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// List returns metadata for all objects matching the given prefix.
// Results are sorted by key name ascending.
func (b *Bucket) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var result []ObjectInfo
	iter := b.bucket.List(&blob.ListOptions{Prefix: prefix})
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		result = append(result, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.ModTime,
		})
	}

	// Sort by key ascending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})

	return result, nil
}

// Head retrieves metadata for a single object without downloading its contents.
// Returns nil and no error if the object does not exist.
func (b *Bucket) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	attrs, err := b.bucket.Attributes(ctx, key)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return nil, nil
		}
		return nil, err
	}
	return &ObjectInfo{
		Key:          key,
		Size:         attrs.Size,
		LastModified: attrs.ModTime,
		ETag:         attrs.ETag,
	}, nil
}

// Download retrieves an object and returns a reader for its contents.
// The caller is responsible for closing the returned ReadCloser.
// Returns an error if the object does not exist.
func (b *Bucket) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	return b.bucket.NewReader(ctx, key, nil)
}

// Close closes the underlying bucket and releases any resources.
func (b *Bucket) Close() error {
	return b.bucket.Close()
}

// ============================================================================
// AWS S3 / S3-Compatible Implementation
// ============================================================================

// S3ClientConfig holds configuration for creating a new S3-compatible storage client.
type S3ClientConfig struct {
	// Endpoint is the S3-compatible endpoint URL (e.g., "https://s3.amazonaws.com" or "https://minio.example.com").
	Endpoint string
	// Bucket is the target bucket name.
	Bucket string
	// Region is the AWS region (e.g., "us-east-1"). Required for AWS S3.
	Region string
	// AccessKeyID is the access key for authentication. If empty, the default credential chain is used.
	AccessKeyID string
	// SecretAccessKey is the secret key for authentication.
	SecretAccessKey string
	// SessionToken is an optional session token for temporary credentials.
	SessionToken string
	// CACert is an optional PEM-encoded CA certificate for custom TLS verification.
	CACert []byte
	// UsePathStyle forces path-style addressing (required for MinIO and some S3-compatible stores).
	UsePathStyle bool
}

// Credentials holds the parsed credentials from a Kubernetes Secret.
type Credentials struct {
	// AccessKeyID is the access key for authentication.
	AccessKeyID string
	// SecretAccessKey is the secret key for authentication.
	SecretAccessKey string
	// SessionToken is an optional session token for temporary credentials.
	SessionToken string
	// Region is an optional region override.
	Region string
	// CACert is an optional PEM-encoded CA certificate.
	CACert []byte
}

// OpenS3Bucket opens an S3-compatible bucket using Go CDK.
// It returns a Bucket wrapper that provides standardized blob operations.
func OpenS3Bucket(ctx context.Context, cfg S3ClientConfig) (*Bucket, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	// Build AWS config
	awsCfg, err := buildAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Create S3 client with custom endpoint configuration
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	// Open bucket using Go CDK s3blob driver
	bucket, err := s3blob.OpenBucketV2(ctx, s3Client, cfg.Bucket, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open S3 bucket: %w", err)
	}

	return NewBucket(bucket), nil
}

// LoadCredentials loads storage credentials from a Kubernetes Secret.
// If secretRef is nil, returns nil (indicating default credential chain should be used).
// If the Secret does not contain the required keys, returns an error.
// The namespace parameter specifies the namespace where the Secret must exist.
// Cross-namespace references are not allowed for security reasons.
func LoadCredentials(ctx context.Context, c client.Client, secretRef *corev1.LocalObjectReference, namespace string) (*Credentials, error) {
	if secretRef == nil {
		return nil, nil
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      secretRef.Name,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials Secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	creds := &Credentials{}

	// Load required credentials (if present - they might use workload identity)
	if v, ok := secret.Data[SecretKeyAccessKeyID]; ok {
		creds.AccessKeyID = string(v)
	}
	if v, ok := secret.Data[SecretKeySecretAccessKey]; ok {
		creds.SecretAccessKey = string(v)
	}

	// Validate that if one key is provided, both must be provided
	if (creds.AccessKeyID != "" && creds.SecretAccessKey == "") ||
		(creds.AccessKeyID == "" && creds.SecretAccessKey != "") {
		return nil, fmt.Errorf("credentials Secret %s/%s must contain both %s and %s, or neither",
			namespace, secretRef.Name, SecretKeyAccessKeyID, SecretKeySecretAccessKey)
	}

	// Load optional fields
	if v, ok := secret.Data[SecretKeySessionToken]; ok {
		creds.SessionToken = string(v)
	}
	if v, ok := secret.Data[SecretKeyRegion]; ok {
		creds.Region = string(v)
	}
	if v, ok := secret.Data[SecretKeyCACert]; ok {
		creds.CACert = v
	}

	return creds, nil
}

// buildAWSConfig constructs AWS SDK config with credentials and custom TLS settings.
func buildAWSConfig(ctx context.Context, cfg S3ClientConfig) (aws.Config, error) {
	var opts []func(*config.LoadOptions) error

	// Set region
	if cfg.Region == "" {
		return aws.Config{}, fmt.Errorf("region is required for S3 client")
	}
	opts = append(opts, config.WithRegion(cfg.Region))

	// Configure credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		staticCreds := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			cfg.SessionToken,
		)
		opts = append(opts, config.WithCredentialsProvider(staticCreds))
	}

	// Configure custom HTTP client for TLS
	httpClient, err := buildHTTPClient(cfg.CACert)
	if err != nil {
		if operatorerrors.IsTransientConnection(err) {
			return aws.Config{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to create HTTP client: %w", err))
		}
		return aws.Config{}, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	opts = append(opts, config.WithHTTPClient(httpClient))

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		if operatorerrors.IsTransientConnection(err) {
			return aws.Config{}, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to load AWS config: %w", err))
		}
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return awsCfg, nil
}

// buildHTTPClient creates an HTTP client with optional custom CA certificate.
func buildHTTPClient(caCert []byte) (*http.Client, error) {
	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
	}

	// Start from the system cert pool when available so that custom CAs are
	// additive instead of replacing the system roots.
	certPool, err := x509.SystemCertPool()
	if err != nil || certPool == nil {
		certPool = x509.NewCertPool()
	}

	if len(caCert) > 0 {
		if !certPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
	}

	transport.TLSClientConfig = &tls.Config{
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   DefaultUploadTimeout,
	}, nil
}
