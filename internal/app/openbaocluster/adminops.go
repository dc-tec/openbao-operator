package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	adminopsapp "github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminops"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

// AdminOpsDependencies holds dependencies required to build admin operations reconcilers.
type AdminOpsDependencies = adminopsapp.Dependencies

// AdminOpsApplication is the prebuilt administrative operations application.
type AdminOpsApplication = adminopsapp.Application

// NewAdminOpsApplication constructs the administrative operations application.
func NewAdminOpsApplication(deps AdminOpsDependencies) *AdminOpsApplication {
	return adminopsapp.NewApplication(
		deps,
		func(
			ctx context.Context,
			logger logr.Logger,
			original *openbaov1alpha1.OpenBaoCluster,
			cluster *openbaov1alpha1.OpenBaoCluster,
			acknowledgements upgrade.RequestAcknowledgements,
			reason string,
		) error {
			return PatchAdminOpsOwnedFieldsWithReader(ctx, deps.APIReader, deps.Client, logger, original, cluster, acknowledgements, reason)
		},
		controllerErrorStatus,
	)
}
