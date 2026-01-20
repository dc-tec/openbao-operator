package interfaces

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

// InitManager handles OpenBao cluster initialization.
// It monitors cluster state and performs initialization when needed.
type InitManager interface {
	// Reconcile checks if the OpenBao cluster needs initialization and initializes it if needed.
	// Returns a Result indicating whether reconciliation should be requeued.
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error)
}
