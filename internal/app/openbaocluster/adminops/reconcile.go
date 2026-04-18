package adminops

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupmanager "github.com/dc-tec/openbao-operator/internal/service/backup"
	inframanager "github.com/dc-tec/openbao-operator/internal/service/infra"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/bluegreen"
	rollingupgrade "github.com/dc-tec/openbao-operator/internal/service/upgrade/rolling"
)

// Dependencies holds dependencies required to build admin operations reconcilers.
type Dependencies struct {
	Client                client.Client
	APIReader             client.Reader
	Scheme                *runtime.Scheme
	Recorder              events.EventRecorder
	OperatorNamespace     string
	OIDCIssuer            string
	OIDCDiscoveryURL      string
	OIDCDiscoveryCAPEM    string
	OIDCJWKSURL           string
	OIDCJWKSCAPEM         string
	OIDCJWTKeys           []string
	SmartClientConfig     portopenbao.ClientConfig
	ImageVerifier         imageverify.Verifier
	OperatorImageVerifier imageverify.Verifier
	RequeueShort          time.Duration
	Platform              string
}

// ErrorRecorder captures errors for metric bookkeeping in controller wrappers.
type ErrorRecorder func(error)

// StatusPatcher persists adminops-owned status fields.
type StatusPatcher func(
	ctx context.Context,
	c client.Client,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	reason string,
) error

// ErrorStatusBuilder creates status payloads from reconciliation errors.
type ErrorStatusBuilder func(error) *openbaov1alpha1.ControllerErrorStatus

type subReconciler interface {
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error)
}

var adminOpsReconcilersBuilder = buildReconcilers

// Reconcile executes admin-operations orchestration and status patching.
func Reconcile(
	ctx context.Context,
	logger logr.Logger,
	deps Dependencies,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	recordError ErrorRecorder,
	patchStatus StatusPatcher,
	errorStatus ErrorStatusBuilder,
) (recon.Result, error) {
	if cluster.Status.AdminOps == nil {
		cluster.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{}
	}

	if errorStatus == nil {
		errorStatus = defaultErrorStatus
	}

	for _, rec := range adminOpsReconcilersBuilder(deps) {
		result, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			if recordError != nil {
				recordError(err)
			}
			cluster.Status.AdminOps.LastError = errorStatus(err)

			if patchStatus != nil {
				if statusErr := patchStatus(ctx, deps.Client, logger, original, cluster, "adminops-error"); statusErr != nil {
					return recon.Result{}, statusErr
				}
			}

			if operatorerrors.IsTransient(err) {
				shouldRequeue, requeueAfter := operatorerrors.ShouldRequeue(err)
				if shouldRequeue {
					if requeueAfter > 0 {
						return recon.Result{RequeueAfter: requeueAfter}, nil
					}
					return recon.Result{RequeueAfter: resolveRequeueShort(deps.RequeueShort)}, nil
				}
			}
			if operatorerrors.IsPermanent(err) {
				return recon.Result{}, err
			}
			return recon.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if patchStatus != nil {
				if statusErr := patchStatus(ctx, deps.Client, logger, original, cluster, "adminops-requeue"); statusErr != nil {
					return recon.Result{}, statusErr
				}
			}
			return recon.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	// Clear previous adminops error after a successful reconcile.
	cluster.Status.AdminOps.LastError = nil
	if patchStatus != nil {
		if err := patchStatus(ctx, deps.Client, logger, original, cluster, "adminops-complete"); err != nil {
			return recon.Result{}, fmt.Errorf("failed to patch adminops owned fields: %w", err)
		}
	}

	return recon.Result{}, nil
}

func buildReconcilers(deps Dependencies) []subReconciler {
	infraMgr := inframanager.NewManagerWithReaderAndOIDCConfig(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		&portauth.OIDCConfig{
			IssuerURL:          deps.OIDCIssuer,
			OIDCDiscoveryURL:   deps.OIDCDiscoveryURL,
			OIDCDiscoveryCAPEM: deps.OIDCDiscoveryCAPEM,
			JWKSURL:            deps.OIDCJWKSURL,
			JWKSCAPEM:          deps.OIDCJWKSCAPEM,
			JWKSKeys:           deps.OIDCJWTKeys,
		},
		deps.Platform,
	)
	backupRuntime := backupmanager.NewUpgradeStrategyRuntime(deps.Client, deps.Scheme)
	adminOpsMutator := func(
		ctx context.Context,
		cluster *openbaov1alpha1.OpenBaoCluster,
		mutate func(obj *openbaov1alpha1.OpenBaoCluster) error,
		forceOwnership bool,
	) error {
		return adminopsstatus.MutateWithReader(ctx, deps.APIReader, deps.Client, cluster, mutate, adminopsstatus.MutateOptions{
			ForceOwnership:  forceOwnership,
			RetryOnConflict: !forceOwnership,
		})
	}

	return []subReconciler{
		bluegreen.NewManager(
			deps.Client,
			deps.Scheme,
			infraMgr,
			backupRuntime,
			deps.SmartClientConfig,
			deps.ImageVerifier,
			deps.OperatorImageVerifier,
			deps.Platform,
			deps.Recorder,
		).WithReader(deps.APIReader),
		rollingupgrade.NewManager(
			deps.Client,
			deps.Scheme,
			backupRuntime,
			deps.SmartClientConfig,
			deps.OperatorImageVerifier,
			deps.Platform,
			deps.Recorder,
		).WithReader(deps.APIReader).WithAdminOpsStatusMutator(adminOpsMutator),
		backupmanager.NewManager(
			deps.Client,
			deps.Scheme,
			deps.SmartClientConfig,
			deps.OperatorImageVerifier,
			deps.Platform,
			deps.Recorder,
		).WithReader(deps.APIReader).WithAdminOpsStatusMutator(adminOpsMutator),
	}
}

func resolveRequeueShort(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 5 * time.Second
}

func defaultErrorStatus(err error) *openbaov1alpha1.ControllerErrorStatus {
	if err == nil {
		return nil
	}

	reason := "Error"
	if mappedReason, ok := operatorerrors.Reason(err); ok {
		reason = mappedReason
	}
	now := metav1.Now()
	return &openbaov1alpha1.ControllerErrorStatus{
		Reason:  reason,
		Message: err.Error(),
		At:      &now,
	}
}
