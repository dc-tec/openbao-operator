package security

import (
	"context"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/go-logr/logr"
)

// VerifyImageForCluster verifies an image reference using the cluster's ImageVerification configuration.
// It returns an image digest reference (e.g., "repo@sha256:...") when verification is enabled and succeeds.
// When image verification is disabled, it returns an empty digest and a nil error.
func VerifyImageForCluster(ctx context.Context, logger logr.Logger, verifier *ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cluster is required")
	}
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}
	if imageRef == "" {
		return "", fmt.Errorf("image reference is required")
	}
	if verifier == nil {
		return "", fmt.Errorf("image verifier is required")
	}

	// Validate that either PublicKey OR (Issuer and Subject) are provided.
	// This matches the validation in ImageVerifier.Verify().
	if cluster.Spec.ImageVerification.PublicKey == "" &&
		(cluster.Spec.ImageVerification.Issuer == "" || cluster.Spec.ImageVerification.Subject == "") {
		return "", fmt.Errorf("image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	config := VerifyConfig{
		PublicKey:        cluster.Spec.ImageVerification.PublicKey,
		Issuer:           cluster.Spec.ImageVerification.Issuer,
		Subject:          cluster.Spec.ImageVerification.Subject,
		IgnoreTlog:       cluster.Spec.ImageVerification.IgnoreTlog,
		ImagePullSecrets: cluster.Spec.ImageVerification.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}

	digest, err := verifier.Verify(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}

// VerifyOperatorImageForCluster verifies an operator-managed helper image (init container,
// backup/upgrade/restore executors) using the cluster's OperatorImageVerification config.
// Unlike VerifyImageForCluster, this function does NOT fall back to ImageVerification.
// If OperatorImageVerification is not configured, verification is skipped for helper images.
func VerifyOperatorImageForCluster(ctx context.Context, logger logr.Logger, verifier *ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cluster is required")
	}
	if imageRef == "" {
		return "", fmt.Errorf("image reference is required")
	}

	// Use OperatorImageVerification only - no fallback to ImageVerification
	// This prevents confusing failures when the main image and helper images have different signers
	verificationConfig := cluster.Spec.OperatorImageVerification
	if verificationConfig == nil || !verificationConfig.Enabled {
		return "", nil
	}

	if verifier == nil {
		return "", fmt.Errorf("image verifier is required")
	}

	// Validate that either PublicKey OR (Issuer and Subject) are provided.
	if verificationConfig.PublicKey == "" &&
		(verificationConfig.Issuer == "" || verificationConfig.Subject == "") {
		return "", fmt.Errorf("operator image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	config := VerifyConfig{
		PublicKey:        verificationConfig.PublicKey,
		Issuer:           verificationConfig.Issuer,
		Subject:          verificationConfig.Subject,
		IgnoreTlog:       verificationConfig.IgnoreTlog,
		ImagePullSecrets: verificationConfig.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}

	digest, err := verifier.Verify(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}
