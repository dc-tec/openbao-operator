package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	"github.com/dc-tec/openbao-operator/internal/security"
)

func imageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ImageVerification == nil {
		return constants.ImageVerificationFailurePolicyBlock
	}
	failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
	if failurePolicy == "" {
		return constants.ImageVerificationFailurePolicyBlock
	}
	return failurePolicy
}

func operatorImageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	config := cluster.Spec.OperatorImageVerification
	if config == nil {
		return constants.ImageVerificationFailurePolicyBlock
	}
	failurePolicy := config.FailurePolicy
	if failurePolicy == "" {
		return constants.ImageVerificationFailurePolicyBlock
	}
	return failurePolicy
}

func initContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.InitContainer == nil {
		return ""
	}
	return cluster.Spec.InitContainer.Image
}

func (m *Manager) verifyImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}
	if imageRef == "" {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := security.VerifyImageForCluster(verifyCtx, logger, m.imageVerifier, cluster, imageRef)
	if err == nil {
		logger.Info("Image verified successfully", "digest", digest)
		return digest, nil
	}

	failurePolicy := imageVerificationFailurePolicy(cluster)
	if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
		return "", operatorerrors.WithReason(failureReason, fmt.Errorf("%s (policy=Block): %w", failureMessagePrefix, err))
	}

	logger.Error(err, failureMessagePrefix+" but proceeding due to Warn policy", "image", imageRef)
	return "", nil
}

func (m *Manager) verifyOperatorImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	// Use OperatorImageVerification only - no fallback to ImageVerification
	verificationConfig := cluster.Spec.OperatorImageVerification
	if verificationConfig == nil || !verificationConfig.Enabled {
		return "", nil
	}
	if imageRef == "" {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := security.VerifyOperatorImageForCluster(verifyCtx, logger, m.operatorImageVerifier, cluster, imageRef)
	if err == nil {
		logger.Info("Operator image verified successfully", "digest", digest)
		return digest, nil
	}

	failurePolicy := operatorImageVerificationFailurePolicy(cluster)
	if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
		return "", operatorerrors.WithReason(failureReason, fmt.Errorf("%s (policy=Block): %w", failureMessagePrefix, err))
	}

	logger.Error(err, failureMessagePrefix+" but proceeding due to Warn policy", "image", imageRef)
	return "", nil
}
