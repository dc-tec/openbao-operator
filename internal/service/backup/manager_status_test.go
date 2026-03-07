package backup

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRecordBackupAttemptPersistsStatus(t *testing.T) {
	cluster := newTestClusterWithBackup("record-attempt", "backup-ns")
	cluster.Status.Backup = nil

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(cluster).
		WithObjects(cluster).
		Build()

	manager := newBackupManager(k8sClient)
	now := time.Unix(1700000000, 0).UTC()
	scheduled := now.Add(5 * time.Minute)
	nextScheduled := scheduled.Add(24 * time.Hour)

	if err := manager.recordBackupAttempt(context.Background(), cluster, now, scheduled, nextScheduled); err != nil {
		t.Fatalf("recordBackupAttempt() error = %v", err)
	}
	if cluster.Status.Backup == nil {
		t.Fatal("cluster.Status.Backup = nil, want initialized")
	}
	if cluster.Status.Backup.LastAttemptTime == nil || !cluster.Status.Backup.LastAttemptTime.Time.Equal(now) {
		t.Fatalf("LastAttemptTime = %#v, want %v", cluster.Status.Backup.LastAttemptTime, now)
	}
	if cluster.Status.Backup.LastAttemptScheduledTime == nil || !cluster.Status.Backup.LastAttemptScheduledTime.Time.Equal(scheduled) {
		t.Fatalf("LastAttemptScheduledTime = %#v, want %v", cluster.Status.Backup.LastAttemptScheduledTime, scheduled)
	}
	if cluster.Status.Backup.NextScheduledBackup == nil || !cluster.Status.Backup.NextScheduledBackup.Time.Equal(nextScheduled) {
		t.Fatalf("NextScheduledBackup = %#v, want %v", cluster.Status.Backup.NextScheduledBackup, nextScheduled)
	}

	updated := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if updated.Status.Backup == nil || updated.Status.Backup.LastAttemptTime == nil || !updated.Status.Backup.LastAttemptTime.Time.Equal(now) {
		t.Fatalf("persisted LastAttemptTime = %#v, want %v", updated.Status.Backup, now)
	}
}
