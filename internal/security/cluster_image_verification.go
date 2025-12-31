package security

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VerifyImageForCluster verifies an image reference using the cluster's ImageVerification configuration.
// It returns an image digest reference (e.g., "repo@sha256:...") when verification is enabled and succeeds.
// When image verification is disabled, it returns an empty digest and a nil error.
func VerifyImageForCluster(ctx context.Context, logger logr.Logger, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	if cluster == nil {
		return "", fmt.Errorf("cluster is required")
	}
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}
	if imageRef == "" {
		return "", fmt.Errorf("image reference is required")
	}

	// Validate that either PublicKey OR (Issuer and Subject) are provided.
	// This matches the validation in ImageVerifier.Verify().
	if cluster.Spec.ImageVerification.PublicKey == "" &&
		(cluster.Spec.ImageVerification.Issuer == "" || cluster.Spec.ImageVerification.Subject == "") {
		return "", fmt.Errorf("image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	verifier := NewImageVerifier(logger, k8sClient, nil)
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
