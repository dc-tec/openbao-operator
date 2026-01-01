package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// SubReconciler is a standardized interface for sub-reconcilers that handle
// specific aspects of OpenBaoCluster reconciliation.
// Implementations should return (shouldRequeue, error) where shouldRequeue
// indicates whether the reconciliation should be requeued immediately.
type SubReconciler interface {
	// Reconcile performs reconciliation for the given cluster.
	// Returns (shouldRequeue, error) where shouldRequeue indicates if
	// reconciliation should be requeued immediately.
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error)
}
