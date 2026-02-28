package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/auth"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
	"github.com/dc-tec/openbao-operator/internal/security"
)

const (
	defaultImageVerificationFailurePolicyBlock = "Block"

	defaultReasonGatewayAPIMissing                   = "GatewayAPIMissing"
	defaultReasonPrerequisitesMissing                = "PrerequisitesMissing"
	defaultReasonACMEDomainNotResolvable             = "ACMEDomainNotResolvable"
	defaultReasonACMEGatewayNotConfiguredPassthrough = "ACMEGatewayNotConfiguredForPassthrough"
	defaultReasonImageVerificationFailed             = "ImageVerificationFailed"
	defaultReasonInitImageVerificationFailed         = "InitContainerImageVerificationFailed"

	infraOpenBaoImageRepoEnv  = "RELATED_IMAGE_OPENBAO"
	infraDefaultOpenBaoImage  = "openbao/openbao"
	infraOpenBaoRevisionLabel = "openbao.org/revision"

	infraImageVerificationTimeout = 5 * time.Second
	infraRequeueShort             = 5 * time.Second
)

// OIDCConfig contains discovered issuer and key material for JWT bootstrap.
type OIDCConfig struct {
	IssuerURL string
	JWKSKeys  []string
}

// InfraReasonPolicy configures infra-related error reason values.
type InfraReasonPolicy struct {
	GatewayAPIMissing                   string
	PrerequisitesMissing                string
	ACMEDomainNotResolvable             string
	ACMEGatewayNotConfiguredPassthrough string
	ImageVerificationFailed             string
	InitContainerImageVerification      string
}

func (p InfraReasonPolicy) gatewayAPIMissingReason() string {
	if strings.TrimSpace(p.GatewayAPIMissing) != "" {
		return p.GatewayAPIMissing
	}
	return defaultReasonGatewayAPIMissing
}

func (p InfraReasonPolicy) prerequisitesMissingReason() string {
	if strings.TrimSpace(p.PrerequisitesMissing) != "" {
		return p.PrerequisitesMissing
	}
	return defaultReasonPrerequisitesMissing
}

func (p InfraReasonPolicy) acmeDomainNotResolvableReason() string {
	if strings.TrimSpace(p.ACMEDomainNotResolvable) != "" {
		return p.ACMEDomainNotResolvable
	}
	return defaultReasonACMEDomainNotResolvable
}

func (p InfraReasonPolicy) acmeGatewayNotConfiguredReason() string {
	if strings.TrimSpace(p.ACMEGatewayNotConfiguredPassthrough) != "" {
		return p.ACMEGatewayNotConfiguredPassthrough
	}
	return defaultReasonACMEGatewayNotConfiguredPassthrough
}

func (p InfraReasonPolicy) imageVerificationFailedReason() string {
	if strings.TrimSpace(p.ImageVerificationFailed) != "" {
		return p.ImageVerificationFailed
	}
	return defaultReasonImageVerificationFailed
}

func (p InfraReasonPolicy) initContainerImageVerificationReason() string {
	if strings.TrimSpace(p.InitContainerImageVerification) != "" {
		return p.InitContainerImageVerification
	}
	return defaultReasonInitImageVerificationFailed
}

type verifyImageFunc func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error)
type verifyOperatorImageFunc func(ctx context.Context, logger logr.Logger, verifier imageverify.Verifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error)
type podClientFactory func(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (openbao.ClusterActions, error)
type discoverOIDCConfigFunc func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error)

// InfraDependencies provides external dependencies for infrastructure reconciliation.
type InfraDependencies struct {
	Client                client.Client
	APIReader             client.Reader
	Scheme                *runtime.Scheme
	RestConfig            *rest.Config
	OperatorNamespace     string
	OIDCIssuer            string
	OIDCJWTKeys           []string
	OperatorImageVerifier imageverify.Verifier
	VerifyImageFunc       verifyImageFunc
	VerifyOperatorImage   verifyOperatorImageFunc
	Recorder              events.EventRecorder
	Platform              string
	SmartClientConfig     openbao.ClientConfig
	ClientForPodFunc      podClientFactory
	DiscoverOIDCConfig    discoverOIDCConfigFunc
}

type infraReconciler struct {
	deps    InfraDependencies
	reasons InfraReasonPolicy
}

// NewInfraReconciler creates a SubReconciler that handles infrastructure orchestration.
func NewInfraReconciler(deps InfraDependencies, reasons InfraReasonPolicy) SubReconciler {
	return &infraReconciler{deps: deps, reasons: reasons}
}

func shouldBootstrapJWTAuth(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.SelfInit != nil &&
		cluster.Spec.SelfInit.OIDC != nil &&
		cluster.Spec.SelfInit.OIDC.Enabled
}

func oidcDiscoveryStatusCode(err error) (int, bool) {
	var statusErr *auth.HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode, true
	}
	return 0, false
}

func oidcDiscoveryError(err error) error {
	if err == nil {
		return nil
	}

	if statusCode, ok := oidcDiscoveryStatusCode(err); ok {
		switch statusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"OIDC discovery blocked by Kubernetes API RBAC (%d). Ensure the operator ServiceAccount can GET %q and %q on the Kubernetes API server (nonResourceURLs RBAC): %w",
				statusCode,
				"/.well-known/openid-configuration",
				"/openid/v1/jwks",
				err,
			))
		case http.StatusNotFound:
			return operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"OIDC discovery endpoint not found (404). Ensure the Kubernetes API server exposes OIDC discovery and JWKS endpoints: %w",
				err,
			))
		default:
			if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
				return operatorerrors.WrapTransientKubernetesAPI(err)
			}
			return operatorerrors.WrapPermanentConfig(err)
		}
	}

	return operatorerrors.WrapTransientKubernetesAPI(operatorerrors.WrapTransientConnection(err))
}

func defaultDiscoverOIDCConfig(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error) {
	discovered, err := auth.DiscoverConfig(ctx, cfg, "")
	if err != nil {
		return nil, err
	}
	if discovered == nil {
		return nil, nil
	}
	return &OIDCConfig{IssuerURL: discovered.IssuerURL, JWKSKeys: discovered.JWKSKeys}, nil
}

func (r *infraReconciler) resolveOIDC(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, []string, error) {
	effectiveIssuer := r.deps.OIDCIssuer
	effectiveKeys := r.deps.OIDCJWTKeys

	if !shouldBootstrapJWTAuth(cluster) || (strings.TrimSpace(effectiveIssuer) != "" && len(effectiveKeys) > 0) {
		return effectiveIssuer, effectiveKeys, nil
	}

	if r.deps.RestConfig == nil {
		return "", nil, operatorerrors.WrapPermanentConfig(fmt.Errorf("OIDC discovery required but controller rest.Config is not available"))
	}

	discover := r.deps.DiscoverOIDCConfig
	if discover == nil {
		discover = defaultDiscoverOIDCConfig
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	discovered, err := discover(discoveryCtx, r.deps.RestConfig)
	if err != nil {
		return "", nil, oidcDiscoveryError(err)
	}
	if discovered == nil || strings.TrimSpace(discovered.IssuerURL) == "" {
		return "", nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("OIDC discovery returned empty issuer"))
	}
	if len(discovered.JWKSKeys) == 0 {
		return "", nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("OIDC discovery returned no JWKS keys"))
	}

	return discovered.IssuerURL, discovered.JWKSKeys, nil
}

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
	if opts.emitEventOnWarn && r.deps.Recorder != nil {
		r.deps.Recorder.Eventf(cluster, nil, corev1.EventTypeWarning, opts.failureReason, opts.failureReason, "%s but proceeding due to Warn policy: %v", opts.failureMessagePrefix, err)
	}
	return "", nil
}

func (r *infraReconciler) verifyMainImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
	opts := imageVerificationOptions{
		enabled:              security.IsMainImageVerificationEnabled(cluster),
		imageRef:             imageRef,
		failurePolicy:        imageVerificationFailurePolicy(cluster),
		failureReason:        r.reasons.imageVerificationFailedReason(),
		failureMessagePrefix: "Image verification failed",
		successMessage:       "Image verified successfully, using digest",
		emitEventOnWarn:      true,
	}

	return r.verifyImageDigestWithPolicy(ctx, logger, cluster, opts, func(ctx context.Context) (string, error) {
		if r.deps.VerifyImageFunc == nil {
			return "", fmt.Errorf("verifyImageFunc is required")
		}
		return r.deps.VerifyImageFunc(ctx, logger, cluster, imageRef)
	})
}

func (r *infraReconciler) verifyOperatorImageDigest(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string, failureReason string, failureMessagePrefix string) (string, error) {
	if !security.IsOperatorImageVerificationEnabled(cluster) {
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

	verifyFunc := r.deps.VerifyOperatorImage
	if verifyFunc == nil {
		verifyFunc = security.VerifyOperatorImageForCluster
	}

	return r.verifyImageDigestWithPolicy(ctx, logger, cluster, opts, func(ctx context.Context) (string, error) {
		return verifyFunc(ctx, logger, r.deps.OperatorImageVerifier, cluster, strings.TrimSpace(imageRef))
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

	return r.verifyOperatorImageDigest(ctx, logger, cluster, initImage, r.reasons.initContainerImageVerificationReason(), "Init container image verification failed")
}

// computeStatefulSetSpec computes the StatefulSetSpec from the cluster and verified image digests.
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

	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		spec.Revision = inframanager.BlueGreenStableRevision(cluster)
		if cluster.Status.BlueGreen != nil &&
			(cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup) {
			logger.Info("Skipping Blue StatefulSet reconciliation during cleanup phase",
				"phase", cluster.Status.BlueGreen.Phase,
				"blueRevision", cluster.Status.BlueGreen.BlueRevision)
			spec.SkipReconciliation = true
			return spec
		}
	} else {
		spec.Revision = ""
	}

	if spec.Revision == "" {
		spec.Name = cluster.Name
	} else {
		spec.Name = fmt.Sprintf("%s-%s", cluster.Name, spec.Revision)
	}

	return spec
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

	if cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen {
		return targetImage
	}

	podReader := client.Reader(r.deps.Client)
	if r.deps.APIReader != nil {
		podReader = r.deps.APIReader
	}

	inframanager.EnsureBlueGreenStatus(ctx, logger, podReader, cluster)

	if cluster.Status.BlueGreen != nil && strings.TrimSpace(cluster.Status.BlueGreen.BlueImage) != "" {
		return strings.TrimSpace(cluster.Status.BlueGreen.BlueImage)
	}
	return targetImage
}

// Reconcile implements the controller's sub-reconciler contract for infrastructure reconciliation.
// nolint:gocyclo
func (r *infraReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error) {
	logger.Info("Reconciling infrastructure for OpenBaoCluster")

	targetImage := r.resolveTargetMainImage(ctx, logger, cluster)

	verifiedImageDigest, err := r.verifyMainImageDigest(ctx, logger, cluster, targetImage)
	if err != nil {
		return recon.Result{}, err
	}
	if strings.TrimSpace(verifiedImageDigest) == "" {
		verifiedImageDigest = targetImage
	}

	verifiedInitContainerDigest, err := r.verifyInitContainerImageDigest(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}
	if strings.TrimSpace(verifiedInitContainerDigest) == "" && cluster.Spec.InitContainer != nil {
		verifiedInitContainerDigest = strings.TrimSpace(cluster.Spec.InitContainer.Image)
	}

	spec := r.computeStatefulSetSpec(logger, cluster, verifiedImageDigest, verifiedInitContainerDigest)

	currentSTS := &appsv1.StatefulSet{}
	if err := r.deps.Client.Get(ctx, client.ObjectKey{Name: spec.Name, Namespace: cluster.Namespace}, currentSTS); err == nil {
		if err := r.handleScaleDownSafety(ctx, cluster, spec.Replicas, currentSTS); err != nil {
			logger.Info("Scale down safety check blocked reconciliation", "reason", err.Error())
			return recon.Result{RequeueAfter: infraRequeueShort}, nil
		}
	}

	effectiveIssuer, effectiveKeys, err := r.resolveOIDC(ctx, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	manager := inframanager.NewManager(r.deps.Client, r.deps.Scheme, r.deps.OperatorNamespace, effectiveIssuer, effectiveKeys, r.deps.Platform)
	if r.deps.APIReader != nil {
		manager = inframanager.NewManagerWithReader(r.deps.Client, r.deps.APIReader, r.deps.Scheme, r.deps.OperatorNamespace, effectiveIssuer, effectiveKeys, r.deps.Platform)
	}
	if err := manager.Reconcile(ctx, logger, cluster, spec); err != nil {
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.gatewayAPIMissingReason(), err)
		}
		if errors.Is(err, inframanager.ErrStatefulSetPrerequisitesMissing) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.prerequisitesMissingReason(), err)
		}
		if errors.Is(err, inframanager.ErrACMEDomainNotResolvable) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.acmeDomainNotResolvableReason(), err)
		}
		if errors.Is(err, inframanager.ErrACMEGatewayNotConfiguredForPassthrough) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.acmeGatewayNotConfiguredReason(), err)
		}
		return recon.Result{}, err
	}

	return recon.Result{}, nil
}

func (r *infraReconciler) handleScaleDownSafety(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, desiredReplicas int32, currentSTS *appsv1.StatefulSet) error {
	if currentSTS.Spec.Replicas == nil {
		return nil
	}
	currentReplicas := *currentSTS.Spec.Replicas
	if desiredReplicas >= currentReplicas {
		return nil
	}

	victimOrdinal := currentReplicas - 1
	victimPodName := fmt.Sprintf("%s-%d", currentSTS.Name, victimOrdinal)

	logger := log.FromContext(ctx).WithValues("victim", victimPodName, "currentReplicas", currentReplicas, "desiredReplicas", desiredReplicas)
	logger.Info("Detected scale down operation; checking victim leadership")

	victimClient, err := r.clientForPod(cluster, victimPodName)
	if err != nil {
		logger.Error(err, "Failed to create client for victim pod; assuming safe to remove")
		return nil
	}

	isLeader, err := victimClient.IsLeader(ctx)
	if err != nil {
		logger.Error(err, "Failed to check leadership of victim pod; assuming safe to remove (pod might be down)")
		return nil
	}

	if isLeader {
		logger.Info("Victim pod is the Active Leader. Attempting graceful step-down.")
		if err := victimClient.StepDownLeader(ctx); err != nil {
			return fmt.Errorf("failed to step down leader %s: %w", victimPodName, err)
		}
		return fmt.Errorf("waiting for leader step-down on %s to complete", victimPodName)
	}

	logger.Info("Victim pod is a follower. Safe to scale down.")
	return nil
}

func (r *infraReconciler) clientForPod(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (openbao.ClusterActions, error) {
	if r.deps.ClientForPodFunc != nil {
		return r.deps.ClientForPodFunc(cluster, podName)
	}

	headlessServiceName := cluster.Name
	podDNS := fmt.Sprintf("%s.%s.%s.svc:8200", podName, headlessServiceName, cluster.Namespace)
	baseURL := "https://" + podDNS

	cfg := r.deps.SmartClientConfig
	cfg.BaseURL = baseURL

	return openbao.NewClient(cfg)
}
