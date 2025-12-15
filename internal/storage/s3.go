package storage

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	operatorerrors "github.com/openbao/operator/internal/errors"
)

const (
	// DefaultUploadTimeout is the default timeout for upload operations.
	DefaultUploadTimeout = 30 * time.Minute
	// MultipartThreshold is the size above which multipart upload is used.
	MultipartThreshold = 100 * 1024 * 1024 // 100 MB
	// DefaultPartSize is the default size of each part in multipart uploads.
	DefaultPartSize = 10 * 1024 * 1024 // 10 MB
	// DefaultConcurrency is the default number of concurrent parts to upload.
	DefaultConcurrency = 3
	// MaxListObjects is the maximum number of objects to return in a single list call.
	MaxListObjects = 1000
)

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
	// PartSize is the size of each part in multipart uploads (in bytes).
	// If zero, DefaultPartSize (10MB) is used.
	PartSize int64
	// Concurrency is the number of concurrent parts to upload during multipart uploads.
	// If zero, DefaultConcurrency (3) is used.
	Concurrency int32
}

// S3Client implements ObjectStorage using AWS SDK v2 for S3-compatible storage.
type S3Client struct {
	client   *s3.Client
	uploader *manager.Uploader
	bucket   string
}

// NewS3Client creates a new S3-compatible storage client.
func NewS3Client(ctx context.Context, cfg S3ClientConfig) (*S3Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket is required")
	}

	// Build AWS config options
	var opts []func(*config.LoadOptions) error

	// Set region
	if cfg.Region == "" {
		return nil, fmt.Errorf("region is required for S3 client")
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
		// Wrap connection errors as transient
		if operatorerrors.IsTransientConnection(err) {
			return nil, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to create HTTP client: %w", err))
		}
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	opts = append(opts, config.WithHTTPClient(httpClient))

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		// Wrap connection errors as transient
		if operatorerrors.IsTransientConnection(err) {
			return nil, operatorerrors.WrapTransientConnection(fmt.Errorf("failed to load AWS config: %w", err))
		}
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})

	// Create uploader with multipart configuration
	partSize := cfg.PartSize
	if partSize == 0 {
		partSize = DefaultPartSize
	}
	concurrency := cfg.Concurrency
	if concurrency == 0 {
		concurrency = DefaultConcurrency
	}
	uploader := manager.NewUploader(s3Client, func(u *manager.Uploader) {
		u.PartSize = partSize
		u.Concurrency = int(concurrency)
	})

	return &S3Client{
		client:   s3Client,
		uploader: uploader,
		bucket:   cfg.Bucket,
	}, nil
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
	var certPool *x509.CertPool
	var err error

	certPool, err = x509.SystemCertPool()
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

// Upload stores the contents of body as an object with the given key.
func (c *S3Client) Upload(ctx context.Context, key string, body io.Reader, size int64) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	}

	// For large objects, use multipart upload via the uploader
	// The uploader handles this automatically based on PartSize
	_, err := c.uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload object %s: %w", key, err)
	}

	return nil
}

// Delete removes the object with the given key.
func (c *S3Client) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}

	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if the error is because the object doesn't exist (not an error for us)
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil
		}
		return fmt.Errorf("failed to delete object %s: %w", key, err)
	}

	return nil
}

// DeleteBatch removes multiple objects at once (up to 1000).
func (c *S3Client) DeleteBatch(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	// S3 DeleteObjects has a limit of 1000 objects per request
	const batchSize = 1000
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		objects := make([]types.ObjectIdentifier, len(batch))
		for j, key := range batch {
			objects[j] = types.ObjectIdentifier{
				Key: aws.String(key),
			}
		}

		_, err := c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects batch: %w", err)
		}
	}

	return nil
}

// List returns metadata for all objects matching the given prefix.
func (c *S3Client) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var result []ObjectInfo
	var continuationToken *string

	for {
		resp, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucket),
			Prefix:            aws.String(prefix),
			MaxKeys:           aws.Int32(MaxListObjects),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list objects with prefix %s: %w", prefix, err)
		}

		for _, obj := range resp.Contents {
			info := ObjectInfo{
				Key:  aws.ToString(obj.Key),
				Size: aws.ToInt64(obj.Size),
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			if obj.ETag != nil {
				info.ETag = *obj.ETag
			}
			result = append(result, info)
		}

		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	// Sort by key ascending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})

	return result, nil
}

// Head retrieves metadata for a single object without downloading its contents.
func (c *S3Client) Head(ctx context.Context, key string) (*ObjectInfo, error) {
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	resp, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if the error is because the object doesn't exist
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return nil, nil
		}
		// Also check for NoSuchKey which some S3-compatible stores return
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get object metadata for %s: %w", key, err)
	}

	info := &ObjectInfo{
		Key:  key,
		Size: aws.ToInt64(resp.ContentLength),
	}
	if resp.LastModified != nil {
		info.LastModified = *resp.LastModified
	}
	if resp.ETag != nil {
		info.ETag = *resp.ETag
	}

	return info, nil
}

// Bucket returns the configured bucket name.
func (c *S3Client) Bucket() string {
	return c.bucket
}
