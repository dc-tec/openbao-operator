package backup

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// ensureBackupServiceAccount creates or updates the ServiceAccount for backup Jobs using Server-Side Apply.
// This ServiceAccount is used for JWT Auth authentication to OpenBao.
func (m *Manager) ensureBackupServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureBackupServiceAccount(ctx, m.client, m.scheme, cluster)
}

// ensureBackupRBAC creates a Role and RoleBinding that grants the backup service account
// permission to list pods in its namespace. This is required for finding the active OpenBao pod.
func (m *Manager) ensureBackupRBAC(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureBackupRBAC(ctx, m.client, m.scheme, cluster)
}

// backupLabels returns the labels for backup resources.
func backupLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentBackup,
	}
}

// backupServiceAccountName returns the name for the backup ServiceAccount.
func backupServiceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixBackupServiceAccount
}
