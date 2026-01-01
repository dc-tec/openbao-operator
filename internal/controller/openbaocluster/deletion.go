package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	controllerutil "github.com/dc-tec/openbao-operator/internal/controller"
	inframanager "github.com/dc-tec/openbao-operator/internal/infra"
)

func (r *OpenBaoClusterReconciler) handleDeletion(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	policy := cluster.Spec.DeletionPolicy
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger.Info("Applying DeletionPolicy for OpenBaoCluster", "deletionPolicy", string(policy))

	// Clear per-cluster metrics to avoid leaving stale series after deletion.
	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.Clear()

	infra := inframanager.NewManager(r.Client, r.Scheme, r.OperatorNamespace, r.OIDCIssuer, r.OIDCJWTKeys)
	if err := infra.Cleanup(ctx, logger, cluster, policy); err != nil {
		return err
	}

	// Backup deletion for DeletionPolicyDeleteAll will be implemented alongside the BackupManager
	// data path. For now, backups are left untouched even when DeleteAll is specified.

	return nil
}
