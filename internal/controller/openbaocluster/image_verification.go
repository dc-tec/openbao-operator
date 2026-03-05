package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
)

// verifyImageRef verifies the signature for the given image reference and returns a digest pin.
// This must verify the same image ref that the infra layer will apply, otherwise we can pin the
// wrong digest (e.g., verifying Green while reconciling Blue during blue/green upgrades).
func (r *OpenBaoClusterReconciler) verifyImageRef(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	if !portsecurity.IsMainImageVerificationEnabled(cluster) {
		return "", nil
	}
	return portsecurity.VerifyImageForCluster(ctx, logger, r.ImageVerifier, cluster, imageRef)
}
