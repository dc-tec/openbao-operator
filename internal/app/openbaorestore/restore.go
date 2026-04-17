package openbaorestore

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
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
	return restoreManagerAdapter{
		manager: restore.NewManager(
			deps.Client,
			deps.Scheme,
			deps.Recorder,
			deps.OperatorImageVerifier,
			deps.Platform,
		).WithReader(deps.APIReader),
	}
}
