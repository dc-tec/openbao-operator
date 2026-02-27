package openbaocluster

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	backupmanager "github.com/dc-tec/openbao-operator/internal/backup"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/dc-tec/openbao-operator/internal/upgrade/bluegreen"
	rollingupgrade "github.com/dc-tec/openbao-operator/internal/upgrade/rolling"
)

// AdminOpsDependencies holds dependencies required to build admin operations reconcilers.
type AdminOpsDependencies struct {
	Client                client.Client
	APIReader             client.Reader
	Scheme                *runtime.Scheme
	OperatorNamespace     string
	OIDCIssuer            string
	OIDCJWTKeys           []string
	SmartClientConfig     openbao.ClientConfig
	ImageVerifier         imageverify.Verifier
	OperatorImageVerifier imageverify.Verifier
	RequeueShort          time.Duration
	Platform              string
}

// ReconcileAdminOps executes admin-operations orchestration and status patching.
func ReconcileAdminOps(
	ctx context.Context,
	logger logr.Logger,
	deps AdminOpsDependencies,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	recordError ErrorRecorder,
) (ctrl.Result, error) {
	if cluster.Status.AdminOps == nil {
		cluster.Status.AdminOps = &openbaov1alpha1.AdminOpsControllerStatus{}
	}

	infraMgr := inframanager.NewManagerWithReader(
		deps.Client,
		deps.APIReader,
		deps.Scheme,
		deps.OperatorNamespace,
		deps.OIDCIssuer,
		deps.OIDCJWTKeys,
		deps.Platform,
	)
	backupRuntime := backupmanager.NewUpgradeStrategyRuntime(deps.Client, deps.Scheme)

	reconcilers := []SubReconciler{
		bluegreen.NewManager(
			deps.Client,
			deps.Scheme,
			infraMgr,
			backupRuntime,
			deps.SmartClientConfig,
			deps.ImageVerifier,
			deps.OperatorImageVerifier,
			deps.Platform,
		),
		rollingupgrade.NewManager(
			deps.Client,
			deps.Scheme,
			backupRuntime,
			deps.SmartClientConfig,
			deps.OperatorImageVerifier,
			deps.Platform,
		),
		backupmanager.NewManager(
			deps.Client,
			deps.Scheme,
			deps.SmartClientConfig,
			deps.OperatorImageVerifier,
			deps.Platform,
		),
	}

	for _, rec := range reconcilers {
		result, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			if recordError != nil {
				recordError(err)
			}
			cluster.Status.AdminOps.LastError = controllerErrorStatus(err)

			if statusErr := PatchAdminOpsOwnedFields(ctx, deps.Client, logger, original, cluster, "adminops-error"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			if operatorerrors.IsTransient(err) {
				shouldRequeue, requeueAfter := operatorerrors.ShouldRequeue(err)
				if shouldRequeue {
					if requeueAfter > 0 {
						return ctrl.Result{RequeueAfter: requeueAfter}, nil
					}
					return ctrl.Result{RequeueAfter: resolveRequeueShort(deps.RequeueShort)}, nil
				}
			}
			if operatorerrors.IsPermanent(err) {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		if result.RequeueAfter > 0 {
			if statusErr := PatchAdminOpsOwnedFields(ctx, deps.Client, logger, original, cluster, "adminops-requeue"); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
		}
	}

	// Clear previous adminops error after a successful reconcile.
	cluster.Status.AdminOps.LastError = nil
	if err := PatchAdminOpsOwnedFields(ctx, deps.Client, logger, original, cluster, "adminops-complete"); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch adminops owned fields: %w", err)
	}

	return ctrl.Result{}, nil
}

func resolveRequeueShort(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 5 * time.Second
}
