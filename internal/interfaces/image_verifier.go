// Package interfaces defines service interfaces for dependency injection.
// This package enables loose coupling between components and facilitates testing.
package interfaces

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// VerifyConfig holds the configuration for image verification.
// Provide PublicKey for static verification OR Issuer/Subject for keyless.
type VerifyConfig struct {
	PublicKey        string
	Issuer           string
	Subject          string
	IgnoreTlog       bool
	ImagePullSecrets []corev1.LocalObjectReference
	Namespace        string
}

// ImageVerifier verifies container image signatures using Cosign.
// Implementations should cache verification results for performance.
type ImageVerifier interface {
	// Verify verifies the signature of the given image reference.
	// Returns the resolved image digest (e.g., "openbao/openbao@sha256:abc...")
	// and an error if verification fails.
	Verify(ctx context.Context, imageRef string, config VerifyConfig) (string, error)
}
