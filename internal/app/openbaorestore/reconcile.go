package openbaorestore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/restore"
)

// ReconcileOpenBaoRestore loads the restore resource and delegates lifecycle
// orchestration to the restore manager.
func ReconcileOpenBaoRestore(
	ctx context.Context,
	c client.Client,
	req ctrl.Request,
	logger logr.Logger,
	restoreManager *restore.Manager,
) (ctrl.Result, error) {
	if restoreManager == nil {
		return ctrl.Result{}, fmt.Errorf("restore manager is required")
	}

	restoreResource := &openbaov1alpha1.OpenBaoRestore{}
	if err := c.Get(ctx, req.NamespacedName, restoreResource); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted - nothing to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get OpenBaoRestore: %w", err)
	}

	logger = logger.WithValues(
		"restore", req.Name,
		"namespace", req.Namespace,
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
