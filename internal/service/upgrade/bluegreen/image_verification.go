package bluegreen

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
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

func resolveInitContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	return inframanager.ResolveInitContainerImage(cluster)
}

func (m *Manager) verifyImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	if !security.IsMainImageVerificationEnabled(cluster) {
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
	return upgrade.VerifyOperatorImageDigest(ctx, logger, m.operatorImageVerifier, cluster, imageRef, failureReason, failureMessagePrefix)
}
