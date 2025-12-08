package storage

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// LoadCredentials loads storage credentials from a Kubernetes Secret.
// If secretRef is nil, returns nil (indicating default credential chain should be used).
// If the Secret does not contain the required keys, returns an error.
func LoadCredentials(ctx context.Context, c client.Client, secretRef *corev1.SecretReference, defaultNamespace string) (*Credentials, error) {
	if secretRef == nil {
		return nil, nil
	}

	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = defaultNamespace
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

// NewS3ClientFromCredentials creates a new S3Client using loaded credentials.
// If creds is nil, the client uses the default credential chain.
func NewS3ClientFromCredentials(ctx context.Context, endpoint, bucket string, creds *Credentials, usePathStyle bool, partSize int64, concurrency int32) (*S3Client, error) {
	cfg := S3ClientConfig{
		Endpoint:     endpoint,
		Bucket:       bucket,
		UsePathStyle: usePathStyle,
		PartSize:     partSize,
		Concurrency:  concurrency,
	}

	if creds != nil {
		cfg.AccessKeyID = creds.AccessKeyID
		cfg.SecretAccessKey = creds.SecretAccessKey
		cfg.SessionToken = creds.SessionToken
		cfg.Region = creds.Region
		cfg.CACert = creds.CACert
	}

	return NewS3Client(ctx, cfg)
}
