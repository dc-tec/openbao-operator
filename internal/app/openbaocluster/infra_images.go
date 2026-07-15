package openbaocluster

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	workloadsvc "github.com/dc-tec/openbao-operator/internal/service/workload"
)

func imageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ImageVerification == nil {
		return defaultImageVerificationFailurePolicyBlock
	}
	failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
	if failurePolicy == "" {
		return defaultImageVerificationFailurePolicyBlock
	}
	return failurePolicy
}

func operatorImageVerificationFailurePolicy(cluster *openbaov1alpha1.OpenBaoCluster) string {
	config := cluster.Spec.OperatorImageVerification
	if config == nil {
		return defaultImageVerificationFailurePolicyBlock
	}
	failurePolicy := config.FailurePolicy
	if failurePolicy == "" {
		return defaultImageVerificationFailurePolicyBlock
	}
	return failurePolicy
}

func defaultIsMainImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}

	if cluster.Spec.ImageVerification != nil {
		return cluster.Spec.ImageVerification.Enabled
	}

	return cluster.Spec.Profile == openbaov1alpha1.ProfileHardened
}

func defaultIsOperatorImageVerificationEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}

	if cluster.Spec.OperatorImageVerification != nil {
		return cluster.Spec.OperatorImageVerification.Enabled
	}

	return cluster.Spec.Profile == openbaov1alpha1.ProfileHardened
}

type imageVerificationOptions struct {
	enabled              bool
	imageRef             string
	failurePolicy        string
	failureReason        string
	failureMessagePrefix string
	successMessage       string
	emitEventOnWarn      bool
}

type resolvedInfraImages struct {
	mainImage string
	initImage string
}

func (r *infraReconciler) verifyImageDigestWithPolicy(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	opts imageVerificationOptions,
	verify func(ctx context.Context) (string, error),
) (string, error) {
	if !opts.enabled {
		return "", nil
	}
	imageRef := strings.TrimSpace(opts.imageRef)
	if imageRef == "" {
		return "", nil
	}
	if verify == nil {
		return "", fmt.Errorf("verify function is required")
	}

	verifyCtx, cancel := context.WithTimeout(ctx, infraImageVerificationTimeout)
	defer cancel()

	digest, err := verify(verifyCtx)
	if err == nil {
		logger.Info(opts.successMessage, "digest", digest)
		return digest, nil
	}

	if opts.failurePolicy == defaultImageVerificationFailurePolicyBlock {
		return "", operatorerrors.WithReason(opts.failureReason, fmt.Errorf("%s (policy=Block): %w", opts.failureMessagePrefix, err))
	}

	logger.Error(err, opts.failureMessagePrefix+" but proceeding due to Warn policy", "image", imageRef)
	if opts.emitEventOnWarn && r.deps.Events.Recorder != nil {
		r.deps.Events.Recorder.Eventf(cluster, nil, corev1.EventTypeWarning, opts.failureReason, opts.failureReason, "%s but proceeding due to Warn policy: %v", opts.failureMessagePrefix, err)
	}
	return "", nil
}

func (r *infraReconciler) verifyMainImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	isMainImageVerificationEnabled := r.deps.ImageVerification.IsMainImageVerificationEnabled
	if isMainImageVerificationEnabled == nil {
		isMainImageVerificationEnabled = defaultIsMainImageVerificationEnabled
	}

	opts := imageVerificationOptions{
		enabled:              isMainImageVerificationEnabled(cluster),
		imageRef:             imageRef,
		failurePolicy:        imageVerificationFailurePolicy(cluster),
		failureReason:        constants.ReasonImageVerificationFailed,
		failureMessagePrefix: "Image verification failed",
		successMessage:       "Image verified successfully, using digest",
		emitEventOnWarn:      true,
	}

	return r.verifyImageDigestWithPolicy(ctx, logger, cluster, opts, func(ctx context.Context) (string, error) {
		if r.deps.ImageVerification.VerifyImageFunc == nil {
			return "", fmt.Errorf("verifyImageFunc is required")
		}
		return r.deps.ImageVerification.VerifyImageFunc(ctx, logger, cluster, imageRef)
	})
}

func (r *infraReconciler) verifyOperatorImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	isOperatorImageVerificationEnabled := r.deps.ImageVerification.IsOperatorImageVerificationEnabled
	if isOperatorImageVerificationEnabled == nil {
		isOperatorImageVerificationEnabled = defaultIsOperatorImageVerificationEnabled
	}
	if !isOperatorImageVerificationEnabled(cluster) {
		return "", nil
	}

	opts := imageVerificationOptions{
		enabled:              true,
		imageRef:             imageRef,
		failurePolicy:        operatorImageVerificationFailurePolicy(cluster),
		failureReason:        failureReason,
		failureMessagePrefix: failureMessagePrefix,
		successMessage:       "Operator image verified successfully",
		emitEventOnWarn:      true,
	}

	verifyFunc := r.deps.ImageVerification.VerifyOperatorImage
	if verifyFunc == nil {
		return "", fmt.Errorf("verifyOperatorImage is required")
	}

	return r.verifyImageDigestWithPolicy(ctx, logger, cluster, opts, func(ctx context.Context) (string, error) {
		return verifyFunc(ctx, logger, r.deps.ImageVerification.OperatorImageVerifier, cluster, strings.TrimSpace(imageRef))
	})
}

func (r *infraReconciler) resolveInitContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	initImage, err := workloadsvc.ResolveInitContainerImage(cluster)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(initImage), nil
}

func (r *infraReconciler) verifyInitContainerImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, initImage string) (string, error) {
	return r.verifyOperatorImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Init container image verification failed")
}

func (r *infraReconciler) resolveVerifiedImages(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (resolvedInfraImages, error) {
	targetImage := r.resolveTargetMainImage(ctx, logger, cluster)

	verifiedMainImage, err := r.verifyMainImageDigest(ctx, logger, cluster, targetImage)
	if err != nil {
		return resolvedInfraImages{}, err
	}
	if strings.TrimSpace(verifiedMainImage) == "" {
		verifiedMainImage = targetImage
	}

	initImage, err := r.resolveInitContainerImage(cluster)
	if err != nil {
		return resolvedInfraImages{}, err
	}

	verifiedInitImage, err := r.verifyInitContainerImageDigest(ctx, logger, cluster, initImage)
	if err != nil {
		return resolvedInfraImages{}, err
	}
	if strings.TrimSpace(verifiedInitImage) == "" {
		verifiedInitImage = initImage
	}

	return resolvedInfraImages{
		mainImage: verifiedMainImage,
		initImage: verifiedInitImage,
	}, nil
}

func defaultOpenBaoImage(specVersion string) string {
	repo := os.Getenv(infraOpenBaoImageRepoEnv)
	if repo == "" {
		repo = infraDefaultOpenBaoImage
	}
	return fmt.Sprintf("%s:%s", repo, strings.TrimSpace(specVersion))
}

// resolveTargetMainImage determines the image reference to use for infrastructure reconciliation.
func (r *infraReconciler) resolveTargetMainImage(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) string {
	targetImage := strings.TrimSpace(cluster.Spec.Image)
	if targetImage == "" {
		targetImage = defaultOpenBaoImage(cluster.Spec.Version)
	}

	if !workloadsvc.IsBlueGreenStrategy(cluster) {
		return targetImage
	}

	podReader := client.Reader(r.deps.Kubernetes.Client)
	if r.deps.Kubernetes.APIReader != nil {
		podReader = r.deps.Kubernetes.APIReader
	}

	workloadsvc.EnsureBlueGreenStatus(ctx, logger, podReader, cluster)

	if cluster.Status.BlueGreen != nil && strings.TrimSpace(cluster.Status.BlueGreen.BlueImage) != "" {
		return strings.TrimSpace(cluster.Status.BlueGreen.BlueImage)
	}
	return targetImage
}
