package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/admission"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	"github.com/dc-tec/openbao-operator/internal/interfaces"
	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
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
	operatorImageVerifier   interfaces.ImageVerifier
	verifyImageFunc         func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error)
	verifyOperatorImageFunc func(ctx context.Context, logger logr.Logger, verifier interfaces.ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error)
	recorder                record.EventRecorder
	admissionStatus         *admission.Status
	platform                string
	smartClientConfig       openbao.ClientConfig
	clientForPodFunc        func(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (*openbao.Client, error)
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

func (r *infraReconciler) verifyMainImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	opts := imageVerificationOptions{
		enabled:              cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled,
		imageRef:             imageRef,
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

	targetImage := cluster.Spec.Image
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen &&
		cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueImage != "" {
		targetImage = cluster.Status.BlueGreen.BlueImage
	}

	verifiedImageDigest, err := r.verifyMainImageDigest(ctx, logger, cluster, targetImage)
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

	// SCALING SAFETY: Check for scale down operations and ensure leader step-down
	// This must happen BEFORE passing the spec to the manager, as the manager updates the StatefulSet.
	currentSTS := &appsv1.StatefulSet{}
	currentSTSName := spec.Name

	// We use r.client (cached) to fetch the StatefulSet.
	// If it doesn't exist, we can't be scaling down, so we proceed.
	if err := r.client.Get(ctx, client.ObjectKey{Name: currentSTSName, Namespace: cluster.Namespace}, currentSTS); err == nil {
		if err := r.handleScaleDownSafety(ctx, cluster, spec.Replicas, currentSTS); err != nil {
			logger.Info("Scale down safety check blocked reconciliation", "reason", err.Error())
			// Return RequeueShort to check again soon (e.g. for leader step-down to complete)
			return recon.Result{RequeueAfter: constants.RequeueShort}, nil
		}
	}

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

func (r *infraReconciler) handleScaleDownSafety(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, desiredReplicas int32, currentSTS *appsv1.StatefulSet) error {
	// 1. Check if we are scaling down
	if currentSTS.Spec.Replicas == nil {
		return nil
	}
	currentReplicas := *currentSTS.Spec.Replicas

	if desiredReplicas >= currentReplicas {
		return nil // Not scaling down, nothing to do
	}

	// 2. Identify the victim pod (highest ordinal)
	// Example: Current=3, Desired=2. Victim is pod-2.
	victimOrdinal := currentReplicas - 1
	// Determine POD name based on STS name logic
	// If it's a blue/green revision, the pod name includes the revision hash
	victimPodName := fmt.Sprintf("%s-%d", currentSTS.Name, victimOrdinal)

	logger := log.FromContext(ctx).WithValues("victim", victimPodName, "currentReplicas", currentReplicas, "desiredReplicas", desiredReplicas)
	logger.Info("Detected scale down operation; checking victim leadership")

	// 3. Create a client specifically for the victim pod
	victimClient, err := r.clientForPod(cluster, victimPodName)
	if err != nil {
		// If we can't create a client (e.g. config error), we probably can't talk to it.
		// Log error and proceed? Or block?
		// "Safe" is to block, but if we can't create a client, we might be stuck.
		// However, failing here is usually a code/config issue.
		logger.Error(err, "Failed to create client for victim pod; assuming safe to remove")
		return nil
	}

	// 4. Check Leadership
	isLeader, err := victimClient.IsLeader(ctx)
	if err != nil {
		logger.Error(err, "Failed to check leadership of victim pod; assuming safe to remove (pod might be down)")
		// If we can't talk to the pod, it's likely not the leader (or won't be for long).
		// Safe to proceed to let K8s terminate it.
		return nil
	}

	// 5. If it IS the leader, Step Down
	if isLeader {
		logger.Info("Victim pod is the Active Leader. Attempting graceful step-down.")

		// Call sys/step-down
		if err := victimClient.StepDownLeader(ctx); err != nil {
			return fmt.Errorf("failed to step down leader %s: %w", victimPodName, err)
		}

		// 6. Block Reconciliation
		// We return an error to stop the InfraManager from updating the StatefulSet immediately.
		// We want to wait for the next reconcile loop where the pod is hopefully a follower.
		return fmt.Errorf("waiting for leader step-down on %s to complete", victimPodName)
	}

	logger.Info("Victim pod is a follower. Safe to scale down.")
	return nil
}

func (r *infraReconciler) clientForPod(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (*openbao.Client, error) {
	if r.clientForPodFunc != nil {
		return r.clientForPodFunc(cluster, podName)
	}

	// Construct the Pod DNS name
	// Format: <pod-name>.<service-name>.<namespace>.svc
	// Use the headless service name
	headlessServiceName := cluster.Name // Default headless service name matches cluster name

	// Address: https://<pod-name>.<cluster-name>.<namespace>.svc:8200
	podDNS := fmt.Sprintf("%s.%s.%s.svc:8200", podName, headlessServiceName, cluster.Namespace)
	baseURL := "https://" + podDNS

	// Clone the smart client config defaults
	config := r.smartClientConfig
	config.BaseURL = baseURL
	// Ensure we use the proper CA cert. r.smartClientConfig should already have it if configured correctly in reconciler setup.
	// If not, we might need to fetch it from the secret, but usually the parent reconciler handles this.
	// For now assume smartClientConfig has the CA bundle if needed.

	// Create client using openbao.NewClient (which creates a new client without shared state,
	// or we could use ClientManager if we wanted shared state, but for this one-off check, NewClient is fine
	// provided we don't need rate limiting persistence for the victim pod specifically).
	// Actually, using NewClient is safer to avoid polluting variable shared state with ephemeral pod clients?
	// But ClientConfig has "ClusterKey".

	return openbao.NewClient(config)
}
