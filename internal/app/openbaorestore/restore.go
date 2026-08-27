package openbaorestore

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/restore"
)

// RestoreReconciler coordinates restore lifecycle transitions for OpenBaoRestore resources.
type RestoreReconciler interface {
	Reconcile(ctx context.Context, logger logr.Logger, restoreResource *openbaov1alpha1.OpenBaoRestore) (recon.Result, error)
}

// RestoreDependencies contains the runtime inputs needed to build the restore reconciler.
type RestoreDependencies struct {
	Client                client.Client
	APIReader             client.Reader
	Scheme                *runtime.Scheme
	Recorder              events.EventRecorder
	OperatorImageVerifier imageverify.Verifier
	Platform              string
	ClientConfig          portopenbao.ClientConfig
}

type restoreManagerAdapter struct {
	manager *restore.Manager
}

func (a restoreManagerAdapter) Reconcile(ctx context.Context, logger logr.Logger, restoreResource *openbaov1alpha1.OpenBaoRestore) (recon.Result, error) {
	result, err := a.manager.Reconcile(ctx, logger, restoreResource)
	return recon.Result{RequeueAfter: result.RequeueAfter}, err
}

// NewRestoreReconciler constructs the restore reconciler used by the controller.
func NewRestoreReconciler(deps RestoreDependencies) RestoreReconciler {
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

	return restoreManagerAdapter{
		manager: restore.NewManager(
			deps.Client,
			deps.Scheme,
			deps.Recorder,
			deps.OperatorImageVerifier,
			deps.Platform,
			deps.ClientConfig,
		).WithReader(deps.APIReader).WithAdminOpsStatusMutator(adminOpsMutator),
	}
}
