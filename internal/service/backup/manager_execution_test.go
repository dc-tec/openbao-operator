package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

func TestExecuteAndProcessBackup_FailedCurrentJobSkipsRetention(t *testing.T) {
	cluster := newTestClusterWithBackup("retention-cluster", "default")
	cluster.Spec.Backup.Retention = &openbaov1alpha1.BackupRetention{MaxCount: 1}
	cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "backup-creds"}

	scheduledTime := time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)
	lastBackupTime := metav1.NewTime(scheduledTime.Add(-24 * time.Hour))
	cluster.Status.Backup.LastBackupTime = &lastBackupTime
	cluster.Status.Backup.LastBackupName = "backups/default/retention-cluster/2025-01-01T03-00-00Z-valid.snap"
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Holder:    backupOperationLockHolder,
	}

	jobName := backupJobName(cluster, scheduledTime)
	failedJob := newBackupJobForCluster(cluster, jobName, scheduledTime)
	failedJob.Annotations = map[string]string{
		"openbao.org/backup-key": "backups/default/retention-cluster/2025-01-02T03-00-00Z-partial.snap",
	}
	failedJob.Status = batchv1.JobStatus{Failed: 1}
	credentials := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: cluster.Namespace},
		Data: map[string][]byte{
			blobstore.SecretKeyAccessKeyID:     []byte("test-ak"),
			blobstore.SecretKeySecretAccessKey: []byte("test-sk"),
		},
	}

	k8sClient := newTestClient(t, cluster, failedJob, credentials)
	manager := newBackupManager(k8sClient)
	store := &fakeBlobStore{objects: []blobstore.ObjectInfo{
		{Key: cluster.Status.Backup.LastBackupName, LastModified: lastBackupTime.Time},
		{Key: failedJob.Annotations["openbao.org/backup-key"], LastModified: scheduledTime},
	}}
	openCalls := 0
	originalOpenBlobStoreFn := openBlobStoreFn
	openBlobStoreFn = func(context.Context, storage.Config) (blobstore.BlobStore, error) {
		openCalls++
		return store, nil
	}
	defer func() { openBlobStoreFn = originalOpenBlobStoreFn }()

	schedule, err := ParseSchedule(cluster.Spec.Backup.Schedule)
	if err != nil {
		t.Fatalf("ParseSchedule() error = %v", err)
	}
	result, err := manager.executeAndProcessBackup(
		context.Background(),
		logr.Discard(),
		cluster,
		schedule,
		NewMetrics(cluster.Namespace, cluster.Name),
		scheduledTime,
		scheduledTime,
		"",
	)
	if err != nil {
		t.Fatalf("executeAndProcessBackup() error = %v", err)
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueShort)
	}
	if openCalls != 0 {
		t.Fatalf("retention storage open calls = %d, want 0", openCalls)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("retention deleted objects after failed Job: %v", store.deleted)
	}
}

type failOnceReader struct {
	client.Reader
	err      error
	getCalls int
}

func (r *failOnceReader) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	r.getCalls++
	if r.getCalls == 1 {
		return r.err
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestCheckBackupDue_RetriesTransientLockReleaseFailure(t *testing.T) {
	cluster := newTestClusterWithBackup("release-cluster", "default")
	completedAt := time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)
	nextScheduled := completedAt.Add(24 * time.Hour)
	lastBackupTime := metav1.NewTime(completedAt)
	backupKey := "backups/default/release-cluster/2025-01-02T03-00-00Z-valid.snap"
	cluster.Status.Backup.LastBackupTime = &lastBackupTime
	cluster.Status.Backup.LastBackupName = backupKey
	cluster.Status.OperationLock = &openbaov1alpha1.OperationLockStatus{
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Holder:    backupOperationLockHolder,
	}

	jobName := backupJobName(cluster, completedAt)
	succeededJob := newBackupJobForCluster(cluster, jobName, completedAt)
	succeededJob.Annotations = map[string]string{"openbao.org/backup-key": backupKey}
	succeededJob.Status = batchv1.JobStatus{Succeeded: 1}

	k8sClient := newTestClient(t, cluster, succeededJob)
	reader := &failOnceReader{Reader: k8sClient, err: errors.New("temporary API read failure")}
	manager := newBackupManager(k8sClient).WithReader(reader)

	shouldReturn, result, err := manager.checkBackupDue(
		context.Background(),
		logr.Discard(),
		cluster,
		completedAt,
		nextScheduled,
		false,
	)
	if err != nil {
		t.Fatalf("checkBackupDue() first call error = %v", err)
	}
	if !shouldReturn {
		t.Fatal("checkBackupDue() first call should return")
	}
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("first RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueShort)
	}
	if cluster.Status.OperationLock == nil {
		t.Fatal("operation lock was cleared after failed release")
	}

	_, _, err = manager.checkBackupDue(
		context.Background(),
		logr.Discard(),
		cluster,
		completedAt,
		nextScheduled,
		false,
	)
	if err != nil {
		t.Fatalf("checkBackupDue() retry error = %v", err)
	}
	if cluster.Status.OperationLock != nil {
		t.Fatalf("operation lock was not cleared on retry: %#v", cluster.Status.OperationLock)
	}
}
