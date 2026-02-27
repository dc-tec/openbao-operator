package imageverify

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// VerifyConfig holds image verification settings.
type VerifyConfig struct {
	PublicKey        string
	Issuer           string
	Subject          string
	IssuerRegExp     string
	SubjectRegExp    string
	IgnoreTlog       bool
	ImagePullSecrets []corev1.LocalObjectReference
	Namespace        string
}

// Verifier validates container image signatures and returns a pinned digest reference.
type Verifier interface {
	Verify(ctx context.Context, imageRef string, config VerifyConfig) (string, error)
}
