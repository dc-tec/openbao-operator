package openbaocluster

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

const (
	defaultImageVerificationFailurePolicyBlock = "Block"

	infraOpenBaoImageRepoEnv = "RELATED_IMAGE_OPENBAO"
	infraDefaultOpenBaoImage = "openbao/openbao"

	infraImageVerificationTimeout = 5 * time.Second
	infraRequeueShort             = 5 * time.Second
)

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
	return constants.ReasonGatewayAPIMissing
}

func (p InfraReasonPolicy) oidcBootstrapConfigurationReason() string {
	if strings.TrimSpace(p.OIDCBootstrapConfiguration) != "" {
		return p.OIDCBootstrapConfiguration
	}
	return constants.ReasonOIDCBootstrapConfigurationInvalid
}

func (p InfraReasonPolicy) apiServerNetworkConfigurationReason() string {
	if strings.TrimSpace(p.APIServerNetworkConfiguration) != "" {
		return p.APIServerNetworkConfiguration
	}
	return constants.ReasonAPIServerNetworkConfigurationInvalid
}

func (p InfraReasonPolicy) prerequisitesMissingReason() string {
	if strings.TrimSpace(p.PrerequisitesMissing) != "" {
		return p.PrerequisitesMissing
	}
	return constants.ReasonPrerequisitesMissing
}

func (p InfraReasonPolicy) acmeDomainNotResolvableReason() string {
	if strings.TrimSpace(p.ACMEDomainNotResolvable) != "" {
		return p.ACMEDomainNotResolvable
	}
	return constants.ReasonACMEDomainNotResolvable
}

func (p InfraReasonPolicy) acmeGatewayNotConfiguredReason() string {
	if strings.TrimSpace(p.ACMEGatewayNotConfiguredPassthrough) != "" {
		return p.ACMEGatewayNotConfiguredPassthrough
	}
	return constants.ReasonACMEGatewayNotConfiguredForPassthrough
}

func (p InfraReasonPolicy) imageVerificationFailedReason() string {
	if strings.TrimSpace(p.ImageVerificationFailed) != "" {
		return p.ImageVerificationFailed
	}
	return constants.ReasonImageVerificationFailed
}

func (p InfraReasonPolicy) initContainerImageVerificationReason() string {
	if strings.TrimSpace(p.InitContainerImageVerification) != "" {
		return p.InitContainerImageVerification
	}
	return constants.ReasonInitContainerImageVerificationFailed
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

	resolvedImages, err := r.resolveVerifiedImages(ctx, logger, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	spec := r.computeStatefulSetSpec(logger, cluster, resolvedImages.mainImage, resolvedImages.initImage)

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

	manager := r.newInfraManager(effectiveOIDC)
	if err := manager.Reconcile(ctx, logger, cluster, spec); err != nil {
		return recon.Result{}, r.mapManagerReconcileError(err)
	}

	return recon.Result{}, nil
}
