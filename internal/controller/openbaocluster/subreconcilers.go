package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

// SubReconciler is a standardized interface for sub-reconcilers that handle
// specific aspects of OpenBaoCluster reconciliation.
// Implementations should return a Result that indicates whether (and when) the
// reconciliation should be requeued.
type SubReconciler interface {
	// Reconcile performs reconciliation for the given cluster.
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error)
}
