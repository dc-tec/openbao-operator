package backup

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestRecordBackupAttemptPersistsStatus(t *testing.T) {
	stored := newTestClusterWithBackup("record-attempt", "backup-ns")
	stored.Status.Backup = nil
	stored.Status.UpgradeRequests = &openbaov1alpha1.UpgradeRequestStatus{
		LastHandledRetry: "persist-me",
	}
	cluster := stored.DeepCopy()
	cluster.Status.UpgradeRequests = nil
	var capturedOptions client.SubResourceApplyOptions
	var sawStatusApply bool

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoCluster{}).
		WithObjects(stored.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceApply: func(ctx context.Context, c client.Client, subResource string, obj runtime.ApplyConfiguration, opts ...client.SubResourceApplyOption) error {
				if subResource == "status" {
					sawStatusApply = true
					capturedOptions = *(&client.SubResourceApplyOptions{}).ApplyOpts(opts)
				}
				return c.Status().Apply(ctx, obj, opts...)
			},
		}).
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
	if updated.Status.UpgradeRequests == nil || updated.Status.UpgradeRequests.LastHandledRetry != "persist-me" {
		t.Fatalf("persisted UpgradeRequests = %#v, want sibling adminops field preserved", updated.Status.UpgradeRequests)
	}
	if cluster.Status.UpgradeRequests == nil || cluster.Status.UpgradeRequests.LastHandledRetry != "persist-me" {
		t.Fatalf("in-memory UpgradeRequests = %#v, want refreshed sibling adminops field", cluster.Status.UpgradeRequests)
	}
	if !sawStatusApply {
		t.Fatal("expected backup status persistence to use status apply")
	}
	if capturedOptions.FieldManager != constants.FieldOwnerAdminOpsStatus {
		t.Fatalf("FieldManager = %q, want %q", capturedOptions.FieldManager, constants.FieldOwnerAdminOpsStatus)
	}
	if capturedOptions.Force != nil && *capturedOptions.Force {
		t.Fatalf("Force = %v, want unset/false", capturedOptions.Force)
	}
}
