package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/revision"
	security "github.com/dc-tec/openbao-operator/internal/security"
)

// infraReconciler wraps InfraManager to implement SubReconciler interface.
// It handles image verification and injects the verified digest into InfraManager.
type infraReconciler struct {
	client                  client.Client
	apiReader               client.Reader
	scheme                  *runtime.Scheme
	operatorNamespace       string
	oidcIssuer              string
	oidcJWTKeys             []string
	operatorImageVerifier   *security.ImageVerifier
	verifyImageFunc         func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error)
	verifyOperatorImageFunc func(ctx context.Context, logger logr.Logger, verifier *security.ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error)
	recorder                record.EventRecorder
	admissionStatus         *admission.Status
	platform                string
}

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
	// Use OperatorImageVerification only - no fallback
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

type imageVerificationOptions struct {
	enabled              bool
	imageRef             string
	failurePolicy        string
	failureReason        string
	failureMessagePrefix string
	successMessage       string
	emitEventOnWarn      bool
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

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := verify(verifyCtx)
	if err == nil {
		logger.Info(opts.successMessage, "digest", digest)
		return digest, nil
	}

	if opts.failurePolicy == constants.ImageVerificationFailurePolicyBlock {
		return "", operatorerrors.WithReason(opts.failureReason, fmt.Errorf("%s (policy=Block): %w", opts.failureMessagePrefix, err))
	}

	logger.Error(err, opts.failureMessagePrefix+" but proceeding due to Warn policy", "image", imageRef)
	if opts.emitEventOnWarn && r.recorder != nil {
		r.recorder.Eventf(cluster, corev1.EventTypeWarning, opts.failureReason, "%s but proceeding due to Warn policy: %v", opts.failureMessagePrefix, err)
	}
	return "", nil
}

func (r *infraReconciler) verifyMainImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	opts := imageVerificationOptions{
		enabled:              cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled,
		imageRef:             cluster.Spec.Image,
		failurePolicy:        imageVerificationFailurePolicy(cluster),
		failureReason:        constants.ReasonImageVerificationFailed,
		failureMessagePrefix: "Image verification failed",
		successMessage:       "Image verified successfully, using digest",
		emitEventOnWarn:      true,
	}

	return r.verifyImageDigestWithPolicy(ctx, logger, cluster, opts, func(ctx context.Context) (string, error) {
		if r.verifyImageFunc == nil {
			return "", fmt.Errorf("verifyImageFunc is required")
		}
		return r.verifyImageFunc(ctx, logger, cluster)
	})
}

func (r *infraReconciler) verifyOperatorImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	// Use OperatorImageVerification only - no fallback to ImageVerification
	verificationConfig := cluster.Spec.OperatorImageVerification
	if verificationConfig == nil || !verificationConfig.Enabled {
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

	verifyFunc := r.verifyOperatorImageFunc
	if verifyFunc == nil {
		verifyFunc = security.VerifyOperatorImageForCluster
	}

	return r.verifyImageDigestWithPolicy(ctx, logger, cluster, opts, func(ctx context.Context) (string, error) {
		return verifyFunc(ctx, logger, r.operatorImageVerifier, cluster, strings.TrimSpace(imageRef))
	})
}

func (r *infraReconciler) verifyInitContainerImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.InitContainer == nil {
		return "", nil
	}

	initImage := strings.TrimSpace(cluster.Spec.InitContainer.Image)
	if initImage == "" {
		return "", nil
	}

	return r.verifyOperatorImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Init container image verification failed")
}

// computeStatefulSetSpec computes the StatefulSetSpec from the cluster and verified image digests.
// This method encapsulates all upgrade strategy knowledge, removing it from the infrastructure layer.
func (r *infraReconciler) computeStatefulSetSpec(
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	verifiedImageDigest string,
	verifiedInitContainerDigest string,
) inframanager.StatefulSetSpec {
	spec := inframanager.StatefulSetSpec{
		Image:              verifiedImageDigest,
		InitContainerImage: verifiedInitContainerDigest,
		Replicas:           cluster.Spec.Replicas,
		DisableSelfInit:    false,
		SkipReconciliation: false,
	}

	// Determine revision and skip logic based on update strategy
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		// For BlueGreen, use the BlueRevision from status
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			spec.Revision = cluster.Status.BlueGreen.BlueRevision

			// Skip StatefulSet creation during DemotingBlue/Cleanup phases.
			// The BlueGreenManager handles StatefulSet lifecycle during these phases.
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				logger.Info("Skipping Blue StatefulSet reconciliation during cleanup phase",
					"phase", cluster.Status.BlueGreen.Phase,
					"blueRevision", cluster.Status.BlueGreen.BlueRevision)
				spec.SkipReconciliation = true
				return spec
			}
		} else {
			// Bootstrap revision for initial reconciliation before BlueGreen status is initialized.
			spec.Revision = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
		}
	} else {
		// For RollingUpdate or default, use empty revision (cluster name)
		spec.Revision = ""
	}

	// Compute StatefulSet name from cluster name and revision
	if spec.Revision == "" {
		spec.Name = cluster.Name
	} else {
		spec.Name = fmt.Sprintf("%s-%s", cluster.Name, spec.Revision)
	}

	// ConfigHash will be computed in the infra manager from the rendered config
	// We leave it empty here and it will be set during reconciliation

	return spec
}

// Reconcile implements SubReconciler for infrastructure reconciliation.
func (r *infraReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	logger.Info("Reconciling infrastructure for OpenBaoCluster")

	verifiedImageDigest, err := r.verifyMainImageDigest(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	verifiedInitContainerDigest, err := r.verifyInitContainerImageDigest(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	// Bootstrap/correct BlueGreen status before infra reconciliation so the infra manager can
	// keep reconciling the active ("Blue") revision even when spec.version/spec.image has been
	// updated to start an upgrade.
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		if cluster.Status.BlueGreen == nil {
			inferred, inferErr := inframanager.InferActiveRevisionFromPods(ctx, r.client, cluster)
			if inferErr != nil {
				logger.Error(inferErr, "Failed to infer active revision from pods; falling back to spec-derived revision")
			}
			blueRevision := inferred
			if blueRevision == "" {
				blueRevision = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
			}
			cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
				Phase:        openbaov1alpha1.PhaseIdle,
				BlueRevision: blueRevision,
			}
		} else if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseIdle &&
			(cluster.Status.BlueGreen.BlueRevision == "" || cluster.Status.CurrentVersion != cluster.Spec.Version) {
			inferred, inferErr := inframanager.InferActiveRevisionFromPods(ctx, r.client, cluster)
			if inferErr != nil {
				logger.Error(inferErr, "Failed to infer active revision from pods; keeping existing BlueRevision", "blueRevision", cluster.Status.BlueGreen.BlueRevision)
			} else if inferred != "" && inferred != cluster.Status.BlueGreen.BlueRevision {
				logger.Info("Correcting BlueRevision from active pods", "from", cluster.Status.BlueGreen.BlueRevision, "to", inferred)
				cluster.Status.BlueGreen.BlueRevision = inferred
			}
		}
	}

	// Compute StatefulSetSpec with all upgrade strategy knowledge
	spec := r.computeStatefulSetSpec(logger, cluster, verifiedImageDigest, verifiedInitContainerDigest)

	manager := inframanager.NewManager(r.client, r.scheme, r.operatorNamespace, r.oidcIssuer, r.oidcJWTKeys, r.platform)
	if r.apiReader != nil {
		manager = inframanager.NewManagerWithReader(r.client, r.apiReader, r.scheme, r.operatorNamespace, r.oidcIssuer, r.oidcJWTKeys, r.platform)
	}
	if err := manager.Reconcile(ctx, logger, cluster, spec); err != nil {
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			return recon.Result{}, operatorerrors.WithReason(ReasonGatewayAPIMissing, err)
		}
		if errors.Is(err, inframanager.ErrStatefulSetPrerequisitesMissing) {
			return recon.Result{}, operatorerrors.WithReason(ReasonPrerequisitesMissing, err)
		}
		if errors.Is(err, inframanager.ErrACMEDomainNotResolvable) {
			return recon.Result{}, operatorerrors.WithReason(ReasonACMEDomainNotResolvable, err)
		}
		if errors.Is(err, inframanager.ErrACMEGatewayNotConfiguredForPassthrough) {
			return recon.Result{}, operatorerrors.WithReason(ReasonACMEGatewayNotConfiguredForPassthrough, err)
		}
		return recon.Result{}, err
	}

	return recon.Result{}, nil
}
