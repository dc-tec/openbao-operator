package openbaorestore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// ReconcileOpenBaoRestore delegates lifecycle orchestration to the restore manager.
func ReconcileOpenBaoRestore(
	ctx context.Context,
	restoreResource *openbaov1alpha1.OpenBaoRestore,
	logger logr.Logger,
	restoreManager RestoreReconciler,
) (recon.Result, error) {
	if restoreManager == nil {
		return recon.Result{}, fmt.Errorf("restore manager is required")
	}
	if restoreResource == nil {
		return recon.Result{}, fmt.Errorf("restore resource is required")
	}

	logger = logger.WithValues(
		"restore", restoreResource.Name,
		"namespace", restoreResource.Namespace,
		"cluster", restoreResource.Spec.Cluster,
		"phase", restoreResource.Status.Phase,
	)
	logger.Info("Reconciling OpenBaoRestore")

	result, err := restoreManager.Reconcile(ctx, logger, restoreResource)
	if err != nil {
		logger.Error(err, "Reconciliation failed")
		return result, err
	}

	if result.RequeueAfter > 0 {
		logger.V(1).Info("Requeuing reconciliation", "requeueAfter", result.RequeueAfter)
	}

	return result, nil
}
