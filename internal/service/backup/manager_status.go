package backup

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
)

// patchStatusSSA updates the backup status using Server-Side Apply.
func (m *Manager) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	applyBackup := func(forceOwnership bool) (*openbaov1alpha1.OpenBaoCluster, error) {
		return statusapply.MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(ctx, m.reader, m.client, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.Backup = cluster.Status.Backup
			return nil
		}, statusapply.OpenBaoClusterAdminOpsStatusApplyOptions{
			ForceOwnership: forceOwnership,
		})
	}

	updated, err := applyBackup(false)
	if err != nil && apierrors.IsConflict(err) {
		// Migration/takeover path: retry with force only when ownership conflict occurs.
		updated, err = applyBackup(true)
	}
	if err != nil {
		return err
	}

	cluster.ResourceVersion = updated.ResourceVersion
	cluster.Status.Upgrade = updated.Status.Upgrade
	cluster.Status.UpgradeRequests = updated.Status.UpgradeRequests
	cluster.Status.Backup = updated.Status.Backup
	cluster.Status.BlueGreen = updated.Status.BlueGreen
	cluster.Status.BreakGlass = updated.Status.BreakGlass
	cluster.Status.AdminOps = updated.Status.AdminOps
	return nil
}

func (m *Manager) recordBackupAttempt(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, now time.Time, scheduledTime time.Time, nextScheduled time.Time) error {
	if cluster.Status.Backup == nil {
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
	}

	nowMeta := metav1.NewTime(now)
	cluster.Status.Backup.LastAttemptTime = &nowMeta

	scheduledMeta := metav1.NewTime(scheduledTime)
	cluster.Status.Backup.LastAttemptScheduledTime = &scheduledMeta

	nextScheduledMeta := metav1.NewTime(nextScheduled)
	cluster.Status.Backup.NextScheduledBackup = &nextScheduledMeta

	return m.patchStatusSSA(ctx, cluster)
}
