package openbaocluster

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

const (
	defaultImageVerificationFailurePolicyBlock = "Block"

	infraOpenBaoImageRepoEnv = "RELATED_IMAGE_OPENBAO"
	infraDefaultOpenBaoImage = "openbao/openbao"

	infraImageVerificationTimeout = 5 * time.Second
	infraRequeueShort             = 5 * time.Second
)

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

// InfraScaleDownRuntime groups authenticated Raft membership operations used
// to stage safe scale-downs.
type InfraScaleDownRuntime struct {
	Runtime initmanagerport.ScaleDownRuntime
}

// InfraDependencies provides external dependencies for infrastructure reconciliation.
type InfraDependencies struct {
	Kubernetes        InfraKubernetesRuntime
	OIDC              InfraOIDCRuntime
	ImageVerification InfraImageVerificationRuntime
	Events            InfraEventRuntime
	Pods              InfraPodRuntime
	ScaleDown         InfraScaleDownRuntime
}

type infraReconciler struct {
	deps InfraDependencies
}

// NewInfraReconciler creates a SubReconciler that handles infrastructure orchestration.
func NewInfraReconciler(deps InfraDependencies) SubReconciler {
	return &infraReconciler{deps: deps}
}

// Reconcile implements the controller's sub-reconciler contract for infrastructure reconciliation.
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
	stagedScaleDown := false

	currentSTS := &appsv1.StatefulSet{}
	reader := r.deps.Kubernetes.APIReader
	if reader == nil {
		reader = r.deps.Kubernetes.Client
	}

	err = reader.Get(ctx, client.ObjectKey{Name: spec.Name, Namespace: cluster.Namespace}, currentSTS)
	switch {
	case err == nil:
		appliedReplicas, err := r.handleScaleDownSafety(ctx, cluster, spec.Replicas, currentSTS)
		if err != nil {
			if operatorerrors.IsPermanent(err) && errors.Is(err, operatorerrors.ErrPermanentPrerequisitesMissing) {
				logger.Info("Scale down safety check requires user intervention", "reason", err.Error())
				return recon.Result{}, nil
			}
			logger.Info("Scale down safety check blocked reconciliation", "reason", err.Error())
			return recon.Result{RequeueAfter: infraRequeueShort}, nil
		}
		spec.Replicas = appliedReplicas
		stagedScaleDown = cluster.Status.Initialized && spec.Replicas > cluster.Spec.Replicas
	case apierrors.IsNotFound(err):
		// No StatefulSet exists yet; scale-down safety only applies once the workload is created.
	default:
		return recon.Result{}, operatorerrors.WrapTransientKubernetesAPI(
			err,
		)
	}

	effectiveOIDC, err := r.resolveOIDC(ctx, cluster)
	if err != nil {
		return recon.Result{}, err
	}

	manager := r.newInfraManager(effectiveOIDC)
	if err := manager.Reconcile(ctx, logger, cluster, spec); err != nil {
		return recon.Result{}, r.mapManagerReconcileError(err)
	}
	if stagedScaleDown {
		logger.Info(
			"Staged scale down step applied; requeueing to continue safe replica reduction",
			"appliedStatefulSetReplicas", spec.Replicas,
			"desiredReplicas", cluster.Spec.Replicas,
		)
		return recon.Result{RequeueAfter: infraRequeueShort}, nil
	}

	return recon.Result{}, nil
}
