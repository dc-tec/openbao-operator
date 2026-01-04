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
	client            client.Client
	scheme            *runtime.Scheme
	operatorNamespace string
	oidcIssuer        string
	oidcJWTKeys       []string
	verifyImageFunc   func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error)
	recorder          record.EventRecorder
	admissionStatus   *admission.Status
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

func (r *infraReconciler) verifyMainImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := r.verifyImageFunc(verifyCtx, logger, cluster)
	if err == nil {
		logger.Info("Image verified successfully, using digest", "digest", digest)
		return digest, nil
	}

	failurePolicy := imageVerificationFailurePolicy(cluster)
	if failurePolicy == constants.ImageVerificationFailurePolicyBlock {
		return "", operatorerrors.WithReason(constants.ReasonImageVerificationFailed, fmt.Errorf("image verification failed (policy=Block): %w", err))
	}

	logger.Error(err, "Image verification failed but proceeding due to Warn policy", "image", cluster.Spec.Image)
	if r.recorder != nil {
		r.recorder.Eventf(cluster, corev1.EventTypeWarning, constants.ReasonImageVerificationFailed, "Image verification failed but proceeding due to Warn policy: %v", err)
	}
	return "", nil
}

func (r *infraReconciler) verifyImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}
	if strings.TrimSpace(imageRef) == "" {
		return "", nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, constants.ImageVerificationTimeout)
	defer cancel()

	digest, err := security.VerifyImageForCluster(verifyCtx, logger, r.client, cluster, imageRef)
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

func (r *infraReconciler) verifySentinelImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.Sentinel == nil || !cluster.Spec.Sentinel.Enabled {
		return "", nil
	}

	sentinelImage := cluster.Spec.Sentinel.Image
	if sentinelImage == "" {
		sentinelImage = inframanager.SentinelImage(cluster, logger)
	}

	return r.verifyImageDigest(ctx, logger, cluster, sentinelImage, constants.ReasonSentinelImageVerificationFailed, "Sentinel image verification failed")
}

func (r *infraReconciler) verifyInitContainerImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.InitContainer == nil {
		return "", nil
	}

	initImage := strings.TrimSpace(cluster.Spec.InitContainer.Image)
	if initImage == "" {
		return "", nil
	}

	return r.verifyImageDigest(ctx, logger, cluster, initImage, constants.ReasonInitContainerImageVerificationFailed, "Init container image verification failed")
}

func (r *infraReconciler) reconcileSentinelAdmissionStatus(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	sentinelAdmissionReady := r.admissionStatus == nil || r.admissionStatus.SentinelReady
	sentinelEnabled := cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled

	if sentinelEnabled && !sentinelAdmissionReady {
		if r.recorder != nil {
			r.recorder.Eventf(cluster, corev1.EventTypeWarning, ReasonAdmissionPoliciesNotReady,
				"Sentinel is enabled but admission policies are not ready; Sentinel will be disabled: %s", r.admissionStatus.SummaryMessage())
		}
		return false
	}

	return sentinelAdmissionReady
}

// Reconcile implements SubReconciler for infrastructure reconciliation.
func (r *infraReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	logger.Info("Reconciling infrastructure for OpenBaoCluster")

	verifiedImageDigest, err := r.verifyMainImageDigest(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	verifiedSentinelDigest, err := r.verifySentinelImageDigest(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	verifiedInitContainerDigest, err := r.verifyInitContainerImageDigest(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	sentinelAdmissionReady := r.reconcileSentinelAdmissionStatus(cluster)

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

	manager := inframanager.NewManagerWithSentinelAdmission(r.client, r.scheme, r.operatorNamespace, r.oidcIssuer, r.oidcJWTKeys, sentinelAdmissionReady)
	if err := manager.Reconcile(ctx, logger, cluster, verifiedImageDigest, verifiedSentinelDigest, verifiedInitContainerDigest); err != nil {
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			return recon.Result{}, operatorerrors.WithReason(ReasonGatewayAPIMissing, err)
		}
		if errors.Is(err, inframanager.ErrStatefulSetPrerequisitesMissing) {
			return recon.Result{}, operatorerrors.WithReason(ReasonPrerequisitesMissing, err)
		}
		return recon.Result{}, err
	}

	return recon.Result{}, nil
}
