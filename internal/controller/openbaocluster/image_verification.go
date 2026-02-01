package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/interfaces"
)

// verifyImageRef verifies the signature for the given image reference and returns a digest pin.
// This must verify the same image ref that the infra layer will apply, otherwise we can pin the
// wrong digest (e.g., verifying Green while reconciling Blue during blue/green upgrades).
func (r *OpenBaoClusterReconciler) verifyImageRef(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
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
	config := interfaces.VerifyConfig{
		PublicKey:        cluster.Spec.ImageVerification.PublicKey,
		Issuer:           cluster.Spec.ImageVerification.Issuer,
		Subject:          cluster.Spec.ImageVerification.Subject,
		IgnoreTlog:       cluster.Spec.ImageVerification.IgnoreTlog,
		ImagePullSecrets: cluster.Spec.ImageVerification.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}
	digest, err := r.ImageVerifier.Verify(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}
