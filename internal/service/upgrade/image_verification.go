package upgrade

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

// OperatorImageVerificationFailurePolicy returns the effective operator image
// verification failure policy for the cluster.
func OperatorImageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.OperatorImageVerification == nil {
		return constants.ImageVerificationFailurePolicyBlock
	}
	if strings.TrimSpace(cluster.Spec.OperatorImageVerification.FailurePolicy) == "" {
		return constants.ImageVerificationFailurePolicyBlock
	}
	return cluster.Spec.OperatorImageVerification.FailurePolicy
}

// VerifyOperatorImageDigest verifies an operator-managed helper image for the
// cluster and applies the effective warn/block failure policy.
func VerifyOperatorImageDigest(
	ctx context.Context,
	logger logr.Logger,
	verifier imageverify.Verifier,
	cluster *openbaov1alpha1.OpenBaoCluster,
	imageRef string,
	failureReason string,
	failureMessagePrefix string,
) (string, error) {
	if !security.IsOperatorImageVerificationEnabled(cluster) {
		return "", nil
	}
	if imageRef == "" {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := security.VerifyOperatorImageForCluster(verifyCtx, logger, verifier, cluster, imageRef)
	if err == nil {
		logger.V(1).Info("Operator image verified successfully", "digest", digest)
		return digest, nil
	}

	if OperatorImageVerificationFailurePolicy(cluster) == constants.ImageVerificationFailurePolicyBlock {
		wrappedErr := fmt.Errorf("%s (policy=Block): %w", failureMessagePrefix, err)
		if failureReason == "" {
			return "", wrappedErr
		}
		return "", operatorerrors.WithReason(failureReason, wrappedErr)
	}

	logger.Error(err, failureMessagePrefix+" but proceeding due to Warn policy", "image", imageRef)
	return "", nil
}
