package openbaorestore

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

type restoreReconciler interface {
	Reconcile(ctx context.Context, logger logr.Logger, restore *openbaov1alpha1.OpenBaoRestore) (recon.Result, error)
}

// ReconcileOpenBaoRestore loads the restore resource and delegates lifecycle
// orchestration to the restore manager.
func ReconcileOpenBaoRestore(
	ctx context.Context,
	c client.Client,
	key types.NamespacedName,
	logger logr.Logger,
	restoreManager restoreReconciler,
) (recon.Result, error) {
	if restoreManager == nil {
		return recon.Result{}, fmt.Errorf("restore manager is required")
	}

	restoreResource := &openbaov1alpha1.OpenBaoRestore{}
	if err := c.Get(ctx, key, restoreResource); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted - nothing to do.
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoRestore: %w", err)
	}

	logger = logger.WithValues(
		"restore", key.Name,
		"namespace", key.Namespace,
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
