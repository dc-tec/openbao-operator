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
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

const (
	defaultImageVerificationFailurePolicyBlock = "Block"

	defaultReasonGatewayAPIMissing                   = "GatewayAPIMissing"
	defaultReasonOIDCBootstrapConfigurationInvalid   = "OIDCBootstrapConfigurationInvalid"
	defaultReasonAPIServerNetworkConfiguration       = "APIServerNetworkConfigurationInvalid"
	defaultReasonPrerequisitesMissing                = "PrerequisitesMissing"
	defaultReasonACMEDomainNotResolvable             = "ACMEDomainNotResolvable"
	defaultReasonACMEGatewayNotConfiguredPassthrough = "ACMEGatewayNotConfiguredForPassthrough"
	defaultReasonImageVerificationFailed             = "ImageVerificationFailed"
	defaultReasonInitImageVerificationFailed         = "InitContainerImageVerificationFailed"

	infraOpenBaoImageRepoEnv = "RELATED_IMAGE_OPENBAO"
	infraDefaultOpenBaoImage = "openbao/openbao"

	infraImageVerificationTimeout = 5 * time.Second
	infraRequeueShort             = 5 * time.Second
)

// OIDCConfig contains discovered issuer and key material for JWT bootstrap.
type OIDCConfig struct {
	IssuerURL          string
	OIDCDiscoveryURL   string
	OIDCDiscoveryCAPEM string
	JWKSURL            string
	JWKSCAPEM          string
	JWKSKeys           []string
}

// InfraReasonPolicy configures infra-related error reason values.
type InfraReasonPolicy struct {
	GatewayAPIMissing                   string
	OIDCBootstrapConfiguration          string
	APIServerNetworkConfiguration       string
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

func (p InfraReasonPolicy) oidcBootstrapConfigurationReason() string {
	if strings.TrimSpace(p.OIDCBootstrapConfiguration) != "" {
		return p.OIDCBootstrapConfiguration
	}
	return defaultReasonOIDCBootstrapConfigurationInvalid
}

func (p InfraReasonPolicy) apiServerNetworkConfigurationReason() string {
	if strings.TrimSpace(p.APIServerNetworkConfiguration) != "" {
		return p.APIServerNetworkConfiguration
	}
	return defaultReasonAPIServerNetworkConfiguration
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
type imageVerificationEnabledFunc func(cluster *openbaov1alpha1.OpenBaoCluster) bool
type oidcDiscoveryStatusCodeFunc func(err error) (int, bool)
type ScaleDownPodClient interface {
	IsLeader(ctx context.Context) (bool, error)
	StepDownLeader(ctx context.Context) error
}
type ScaleDownPodClientFactory func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (ScaleDownPodClient, error)
type discoverOIDCConfigFunc func(ctx context.Context, cfg *rest.Config) (*OIDCConfig, error)

// InfraKubernetesRuntime groups Kubernetes-facing collaborators used by infra reconciliation.
type InfraKubernetesRuntime struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Platform          string
}

// InfraOIDCRuntime groups OIDC discovery inputs and warmup state.
type InfraOIDCRuntime struct {
	RestConfig          *rest.Config
	OIDCIssuer          string
	OIDCDiscoveryURL    string
	OIDCDiscoveryCAPEM  string
	OIDCJWKSURL         string
	OIDCJWKSCAPEM       string
	OIDCJWTKeys         []string
	DiscoverOIDCConfig  discoverOIDCConfigFunc
	DiscoveryStatusCode oidcDiscoveryStatusCodeFunc
}

// InfraImageVerificationRuntime groups image verification collaborators.
type InfraImageVerificationRuntime struct {
	OperatorImageVerifier              imageverify.Verifier
	VerifyImageFunc                    verifyImageFunc
	VerifyOperatorImage                verifyOperatorImageFunc
	IsMainImageVerificationEnabled     imageVerificationEnabledFunc
	IsOperatorImageVerificationEnabled imageVerificationEnabledFunc
}

// InfraEventRuntime groups event emission dependencies.
type InfraEventRuntime struct {
	Recorder events.EventRecorder
}

// InfraPodRuntime groups pod-targeted OpenBao client construction.
type InfraPodRuntime struct {
	ClientForPodFunc ScaleDownPodClientFactory
}

// InfraDependencies provides external dependencies for infrastructure reconciliation.
type InfraDependencies struct {
	Kubernetes        InfraKubernetesRuntime
	OIDC              InfraOIDCRuntime
	ImageVerification InfraImageVerificationRuntime
	Events            InfraEventRuntime
	Pods              InfraPodRuntime
}

type infraReconciler struct {
	deps    InfraDependencies
	reasons InfraReasonPolicy
}

// NewInfraReconciler creates a SubReconciler that handles infrastructure orchestration.
func NewInfraReconciler(deps InfraDependencies, reasons InfraReasonPolicy) SubReconciler {
	return &infraReconciler{deps: deps, reasons: reasons}
}

func (r *infraReconciler) oidcBootstrapConfigurationError(err error) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, operatorerrors.ErrPermanentConfig) {
		err = operatorerrors.WrapPermanentConfig(err)
	}
	return operatorerrors.WithReason(r.reasons.oidcBootstrapConfigurationReason(), err)
}

func shouldBootstrapJWTAuth(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return portauth.OperatorJWTBootstrapEnabled(cluster)
}

func (r *infraReconciler) oidcDiscoveryStatusCode(err error) (int, bool) {
	if r == nil || r.deps.OIDC.DiscoveryStatusCode == nil {
		return 0, false
	}
	return r.deps.OIDC.DiscoveryStatusCode(err)
}

func (r *infraReconciler) oidcDiscoveryError(err error) error {
	if err == nil {
		return nil
	}

	if statusCode, ok := r.oidcDiscoveryStatusCode(err); ok {
		switch statusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return r.oidcBootstrapConfigurationError(fmt.Errorf(
				"OIDC discovery blocked by Kubernetes API RBAC (%d). Ensure the operator ServiceAccount can GET %q and %q on the Kubernetes API server (nonResourceURLs RBAC): %w",
				statusCode,
				"/.well-known/openid-configuration",
				"/openid/v1/jwks",
				err,
			))
		case http.StatusNotFound:
			return r.oidcBootstrapConfigurationError(fmt.Errorf(
				"OIDC discovery endpoint not found (404). Ensure the Kubernetes API server exposes OIDC discovery and JWKS endpoints: %w",
				err,
			))
		default:
			if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
				return operatorerrors.WrapTransientKubernetesAPI(err)
			}
			return r.oidcBootstrapConfigurationError(err)
		}
	}

	return operatorerrors.WrapTransientKubernetesAPI(operatorerrors.WrapTransientConnection(err))
}

func (r *infraReconciler) resolveOIDC(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*OIDCConfig, error) {
	effective := &OIDCConfig{
		IssuerURL:          r.deps.OIDC.OIDCIssuer,
		OIDCDiscoveryURL:   r.deps.OIDC.OIDCDiscoveryURL,
		OIDCDiscoveryCAPEM: r.deps.OIDC.OIDCDiscoveryCAPEM,
		JWKSURL:            r.deps.OIDC.OIDCJWKSURL,
		JWKSCAPEM:          r.deps.OIDC.OIDCJWKSCAPEM,
		JWKSKeys:           append([]string(nil), r.deps.OIDC.OIDCJWTKeys...),
	}

	if !shouldBootstrapJWTAuth(cluster) || (strings.TrimSpace(effective.IssuerURL) != "" && (strings.TrimSpace(effective.OIDCDiscoveryURL) != "" || strings.TrimSpace(effective.JWKSURL) != "" || len(effective.JWKSKeys) > 0)) {
		return effective, nil
	}

	if r.deps.OIDC.RestConfig == nil {
		return nil, r.oidcBootstrapConfigurationError(fmt.Errorf("OIDC discovery required but controller rest.Config is not available"))
	}

	discover := r.deps.OIDC.DiscoverOIDCConfig
	if discover == nil {
		return nil, r.oidcBootstrapConfigurationError(fmt.Errorf("OIDC discovery function is not configured"))
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	discovered, err := discover(discoveryCtx, r.deps.OIDC.RestConfig)
	if err != nil {
		return nil, r.oidcDiscoveryError(err)
	}
	if discovered == nil || strings.TrimSpace(discovered.IssuerURL) == "" {
		return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("OIDC discovery returned empty issuer"))
	}
	if strings.TrimSpace(discovered.OIDCDiscoveryURL) == "" && strings.TrimSpace(discovered.JWKSURL) == "" && len(discovered.JWKSKeys) == 0 {
		return nil, operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("OIDC discovery returned no JWT validation material"))
	}

	return discovered, nil
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
		failureReason:        r.reasons.imageVerificationFailedReason(),
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

	podReader := client.Reader(r.deps.Kubernetes.Client)
	if r.deps.Kubernetes.APIReader != nil {
		podReader = r.deps.Kubernetes.APIReader
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

	if err := upgrade.ValidateUpgradeTargetVersion(logger, cluster.Status.CurrentVersion, cluster.Spec.Version); err != nil {
		return recon.Result{}, err
	}
	if err := upgrade.ValidateImageRefMatchesVersion(cluster.Spec.Version, cluster.Spec.Image); err != nil {
		return recon.Result{}, err
	}

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
	if err := r.deps.Kubernetes.Client.Get(ctx, client.ObjectKey{Name: spec.Name, Namespace: cluster.Namespace}, currentSTS); err == nil {
		if err := r.handleScaleDownSafety(ctx, cluster, spec.Replicas, currentSTS); err != nil {
			logger.Info("Scale down safety check blocked reconciliation", "reason", err.Error())
			return recon.Result{RequeueAfter: infraRequeueShort}, nil
		}
	}

	effectiveOIDC, err := r.resolveOIDC(ctx, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	manager := inframanager.NewManager(
		r.deps.Kubernetes.Client,
		r.deps.Kubernetes.Scheme,
		r.deps.Kubernetes.OperatorNamespace,
		effectiveOIDC.IssuerURL,
		effectiveOIDC.JWKSKeys,
		r.deps.Kubernetes.Platform,
	)
	manager.SetOIDCConfig(&portauth.OIDCConfig{
		IssuerURL:          effectiveOIDC.IssuerURL,
		OIDCDiscoveryURL:   effectiveOIDC.OIDCDiscoveryURL,
		OIDCDiscoveryCAPEM: effectiveOIDC.OIDCDiscoveryCAPEM,
		JWKSURL:            effectiveOIDC.JWKSURL,
		JWKSCAPEM:          effectiveOIDC.JWKSCAPEM,
		JWKSKeys:           effectiveOIDC.JWKSKeys,
	})
	if r.deps.Kubernetes.APIReader != nil {
		manager = inframanager.NewManagerWithReader(
			r.deps.Kubernetes.Client,
			r.deps.Kubernetes.APIReader,
			r.deps.Kubernetes.Scheme,
			r.deps.Kubernetes.OperatorNamespace,
			effectiveOIDC.IssuerURL,
			effectiveOIDC.JWKSKeys,
			r.deps.Kubernetes.Platform,
		)
		manager.SetOIDCConfig(&portauth.OIDCConfig{
			IssuerURL:          effectiveOIDC.IssuerURL,
			OIDCDiscoveryURL:   effectiveOIDC.OIDCDiscoveryURL,
			OIDCDiscoveryCAPEM: effectiveOIDC.OIDCDiscoveryCAPEM,
			JWKSURL:            effectiveOIDC.JWKSURL,
			JWKSCAPEM:          effectiveOIDC.JWKSCAPEM,
			JWKSKeys:           effectiveOIDC.JWKSKeys,
		})
	}
	if err := manager.Reconcile(ctx, logger, cluster, spec); err != nil {
		if errors.Is(err, inframanager.ErrOIDCBootstrapAudienceMismatch) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.oidcBootstrapConfigurationReason(), err)
		}
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.gatewayAPIMissingReason(), err)
		}
		if errors.Is(err, inframanager.ErrAPIServerNetworkConfigurationInvalid) {
			return recon.Result{}, operatorerrors.WithReason(r.reasons.apiServerNetworkConfigurationReason(), err)
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

	victimClient, err := r.clientForPod(ctx, cluster, victimPodName)
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

func (r *infraReconciler) clientForPod(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (ScaleDownPodClient, error) {
	if r.deps.Pods.ClientForPodFunc != nil {
		return r.deps.Pods.ClientForPodFunc(ctx, cluster, podName)
	}
	return nil, fmt.Errorf("OpenBao pod client factory is not configured")
}
