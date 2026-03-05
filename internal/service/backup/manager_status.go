package backup

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
)

// patchStatusSSA updates the backup status using Server-Side Apply.
func (m *Manager) patchStatusSSA(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Backup: cluster.Status.Backup,
		},
	}

	applyConfig, err := kube.ToApplyConfiguration(applyCluster, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	return m.client.Status().Apply(ctx, applyConfig,
		client.FieldOwner("openbao-adminops-controller"),
		client.ForceOwnership,
	)
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
