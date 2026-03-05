package security

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	internalsecurity "github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

// NewImageVerifier creates the default cluster image verifier implementation.
func NewImageVerifier(logger logr.Logger, k8sClient client.Client) imageverify.Verifier {
	return internalsecurity.NewImageVerifier(logger, k8sClient, nil)
}

// VerifyImageForCluster verifies a main workload image using cluster policy.
func VerifyImageForCluster(ctx context.Context, logger logr.Logger, verifier imageverify.Verifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	return internalsecurity.VerifyImageForCluster(ctx, logger, verifier, cluster, imageRef)
}

// VerifyOperatorImageForCluster verifies an operator-managed helper image using cluster policy.
func VerifyOperatorImageForCluster(ctx context.Context, logger logr.Logger, verifier imageverify.Verifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	return internalsecurity.VerifyOperatorImageForCluster(ctx, logger, verifier, cluster, imageRef)
}

// IsMainImageVerificationEnabled reports whether main image verification is enabled.
func IsMainImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return internalsecurity.IsMainImageVerificationEnabled(cluster)
}

// IsOperatorImageVerificationEnabled reports whether operator image verification is enabled.
func IsOperatorImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return internalsecurity.IsOperatorImageVerificationEnabled(cluster)
}
