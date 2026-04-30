package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/deletionops"
)

// DeletionDependencies defines dependencies for OpenBaoCluster deletion orchestration.
type DeletionDependencies = deletionops.Dependencies

// HandleDeletion applies deletion policy side effects.
func HandleDeletion(ctx context.Context, logger logr.Logger, deps DeletionDependencies, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if len(deps.RetentionSecrets) == 0 {
		deps.RetentionSecrets = deletionops.DefaultRetentionSecrets(cluster)
	}
	return deletionops.Handle(ctx, logger, deps, cluster)
}
