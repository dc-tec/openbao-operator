package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	security "github.com/dc-tec/openbao-operator/internal/security"
)

// verifyImage verifies the container image signature using the ImageVerifier.
// Returns the verified image digest that should be used in the StatefulSet.
// Supports both static key verification (PublicKey) and keyless verification (Issuer + Subject).
func (r *OpenBaoClusterReconciler) verifyImage(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}

	// Validate that either PublicKey OR (Issuer and Subject) are provided
	// This matches the validation in ImageVerifier.Verify()
	if cluster.Spec.ImageVerification.PublicKey == "" && (cluster.Spec.ImageVerification.Issuer == "" || cluster.Spec.ImageVerification.Subject == "") {
		return "", fmt.Errorf("image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	// Use the singleton ImageVerifier (initialized in SetupWithManager)
	// This ensures we benefit from the internal LRU and TTL caches.
	config := security.VerifyConfig{
		PublicKey:        cluster.Spec.ImageVerification.PublicKey,
		Issuer:           cluster.Spec.ImageVerification.Issuer,
		Subject:          cluster.Spec.ImageVerification.Subject,
		IgnoreTlog:       cluster.Spec.ImageVerification.IgnoreTlog,
		ImagePullSecrets: cluster.Spec.ImageVerification.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}
	digest, err := r.ImageVerifier.Verify(ctx, cluster.Spec.Image, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}
