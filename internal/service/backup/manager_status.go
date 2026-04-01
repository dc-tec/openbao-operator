package backup

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
)

// patchStatusSSA updates the backup status using Server-Side Apply.
func (m *Manager) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return statusapply.ApplyOpenBaoClusterAdminOpsStatus(ctx, m.client, cluster, statusapply.OpenBaoClusterAdminOpsStatusApplyOptions{
		ForceOwnership: true,
	})
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
