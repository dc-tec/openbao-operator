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

// CanContinueWithoutAdmission reports whether reconciliation can only observe
// and drain an execution that crossed its durable creation boundary. The
// restore manager does not create a Job from any of these states.
func CanContinueWithoutAdmission(restoreResource *openbaov1alpha1.OpenBaoRestore) bool {
	if restoreResource == nil || restoreResource.Status.Execution == nil {
		return false
	}

	switch restoreResource.Status.Execution.Stage {
	case openbaov1alpha1.RestoreExecutionStageCommitted,
		openbaov1alpha1.RestoreExecutionStageCreated,
		openbaov1alpha1.RestoreExecutionStageTerminalObserved,
		openbaov1alpha1.RestoreExecutionStageFollowThroughComplete,
		openbaov1alpha1.RestoreExecutionStageUnknown:
		return restoreResource.Status.Phase == openbaov1alpha1.RestorePhaseRunning ||
			restoreResource.Status.Phase == openbaov1alpha1.RestorePhaseCompleted ||
			restoreResource.Status.Phase == openbaov1alpha1.RestorePhaseFailed ||
			restoreResource.Status.Phase == openbaov1alpha1.RestorePhaseUnknown
	default:
		return false
	}
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
	adminOpsMutator := adminopsstatus.NewMutator(deps.APIReader, deps.Client)

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
