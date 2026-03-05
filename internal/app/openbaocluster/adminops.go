package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	adminopsapp "github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminops"
	recon "github.com/dc-tec/openbao-operator/internal/reconcile"
)

// AdminOpsDependencies holds dependencies required to build admin operations reconcilers.
type AdminOpsDependencies = adminopsapp.Dependencies

// ReconcileAdminOps executes admin-operations orchestration and status patching.
func ReconcileAdminOps(
	ctx context.Context,
	logger logr.Logger,
	deps AdminOpsDependencies,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	recordError ErrorRecorder,
) (recon.Result, error) {
	return adminopsapp.Reconcile(
		ctx,
		logger,
		deps,
		original,
		cluster,
		adminopsapp.ErrorRecorder(recordError),
		PatchAdminOpsOwnedFields,
		controllerErrorStatus,
	)
}
