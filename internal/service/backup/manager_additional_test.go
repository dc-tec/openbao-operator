package backup

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/storage"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

type metricsBlobStore struct {
	headInfo   *blobstore.ObjectInfo
	headErr    error
	closeCount int
}

type capturingLogSink struct {
	infoCount  int
	errorCount int
}

func (c *capturingLogSink) Init(logr.RuntimeInfo) {}

func (c *capturingLogSink) Enabled(int) bool { return true }

func (c *capturingLogSink) Info(int, string, ...interface{}) {
	c.infoCount++
}

func (c *capturingLogSink) Error(error, string, ...interface{}) {
	c.errorCount++
}

func (c *capturingLogSink) WithValues(...interface{}) logr.LogSink { return c }

func (c *capturingLogSink) WithName(string) logr.LogSink { return c }

func (m *metricsBlobStore) Upload(context.Context, string, io.Reader) error { return nil }

func (m *metricsBlobStore) Download(context.Context, string) (io.ReadCloser, error) { return nil, nil }

func (m *metricsBlobStore) Delete(context.Context, string) error { return nil }

func (m *metricsBlobStore) DeleteBatch(context.Context, []string) error { return nil }

func (m *metricsBlobStore) List(context.Context, string) ([]blobstore.ObjectInfo, error) {
	return nil, nil
}

func (m *metricsBlobStore) Head(context.Context, string) (*blobstore.ObjectInfo, error) {
	return m.headInfo, m.headErr
}

func (m *metricsBlobStore) Close() error {
	m.closeCount++
	return nil
}

func resetBackupTestState(namespace, name string) {
	NewMetrics(namespace, name).Clear()
	backupJobMetricsSeen = sync.Map{}
}

func newBackupManager(client client.Client) *Manager {
	return &Manager{
		client: client,
		scheme: testScheme,
	}
}

func newBackupJobForCluster(cluster *openbaov1alpha1.OpenBaoCluster, name string, createdAt time.Time) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         cluster.Namespace,
			UID:               types.UID(name),
			CreationTimestamp: metav1.NewTime(createdAt),
			Labels:            backupLabels(cluster),
		},
	}
}

func TestSyncBackupStatusMetricsReflectsClusterStatus(t *testing.T) {
	cluster := newTestClusterWithBackup("status-metrics", "backup-ns")
	attemptTime := metav1.NewTime(time.Unix(1700000100, 0))
	successTime := metav1.NewTime(time.Unix(1700000200, 0))
	cluster.Status.Backup = &openbaov1alpha1.BackupStatus{
		ConsecutiveFailures: 3,
		LastAttemptTime:     &attemptTime,
		LastBackupTime:      &successTime,
		LastBackupSize:      4096,
		LastBackupDuration:  "45s",
	}

	metrics := NewMetrics(cluster.Namespace, cluster.Name)
	resetBackupTestState(cluster.Namespace, cluster.Name)
	defer metrics.Clear()

	newBackupManager(newTestClient(t)).syncBackupStatusMetrics(cluster, metrics)

	if got := testutil.ToFloat64(backupConsecutiveFailures.WithLabelValues(cluster.Namespace, cluster.Name)); got != 3 {
		t.Fatalf("backupConsecutiveFailures = %v, want 3", got)
	}
	if got := testutil.ToFloat64(backupLastAttemptTimestamp.WithLabelValues(cluster.Namespace, cluster.Name)); got != 1700000100 {
		t.Fatalf("backupLastAttemptTimestamp = %v, want 1700000100", got)
	}
	if got := testutil.ToFloat64(backupLastSuccessTimestamp.WithLabelValues(cluster.Namespace, cluster.Name)); got != 1700000200 {
		t.Fatalf("backupLastSuccessTimestamp = %v, want 1700000200", got)
	}
	if got := testutil.ToFloat64(backupLastSizeBytes.WithLabelValues(cluster.Namespace, cluster.Name)); got != 4096 {
		t.Fatalf("backupLastSizeBytes = %v, want 4096", got)
	}
	if got := testutil.ToFloat64(backupLastDurationSeconds.WithLabelValues(cluster.Namespace, cluster.Name)); got != 45 {
		t.Fatalf("backupLastDurationSeconds = %v, want 45", got)
	}
}

func TestCollectBackupJobMetricsSnapshotUpdatesMetricsAndDeduplicates(t *testing.T) {
	cluster := newTestClusterWithBackup("snapshot-cluster", "backup-ns")
	succeededAt := time.Unix(1700001000, 0)
	failedAt := time.Unix(1700002000, 0)

	succeeded := newBackupJobForCluster(cluster, "backup-success", time.Unix(1700000900, 0))
	succeeded.Status.Succeeded = 1
	succeeded.Status.CompletionTime = ptrToTime(succeededAt)

	failed := newBackupJobForCluster(cluster, "backup-failed", time.Unix(1700001900, 0))
	failed.Status.Failed = 1
	failed.Status.CompletionTime = ptrToTime(failedAt)

	running := newBackupJobForCluster(cluster, "backup-running", time.Unix(1700003000, 0))
	running.Status.Active = 1

	manager := newBackupManager(newTestClient(t, succeeded, failed, running))
	metrics := NewMetrics(cluster.Namespace, cluster.Name)
	resetBackupTestState(cluster.Namespace, cluster.Name)
	defer metrics.Clear()

	snapshot, err := manager.collectBackupJobMetricsSnapshot(context.Background(), cluster, metrics)
	if err != nil {
		t.Fatalf("collectBackupJobMetricsSnapshot() error = %v", err)
	}

	if !snapshot.inProgress {
		t.Fatal("snapshot.inProgress = false, want true")
	}
	if snapshot.newestSucceeded == nil || snapshot.newestSucceeded.Name != succeeded.Name {
		t.Fatalf("snapshot.newestSucceeded = %#v, want %q", snapshot.newestSucceeded, succeeded.Name)
	}
	if snapshot.newestFailed == nil || snapshot.newestFailed.Name != failed.Name {
		t.Fatalf("snapshot.newestFailed = %#v, want %q", snapshot.newestFailed, failed.Name)
	}
	if got := testutil.ToFloat64(backupSuccessTotal.WithLabelValues(cluster.Namespace, cluster.Name)); got != 1 {
		t.Fatalf("backupSuccessTotal = %v, want 1", got)
	}
	if got := testutil.ToFloat64(backupFailureTotal.WithLabelValues(cluster.Namespace, cluster.Name)); got != 1 {
		t.Fatalf("backupFailureTotal = %v, want 1", got)
	}

	if _, err := manager.collectBackupJobMetricsSnapshot(context.Background(), cluster, metrics); err != nil {
		t.Fatalf("second collectBackupJobMetricsSnapshot() error = %v", err)
	}
	if got := testutil.ToFloat64(backupSuccessTotal.WithLabelValues(cluster.Namespace, cluster.Name)); got != 1 {
		t.Fatalf("backupSuccessTotal after dedupe = %v, want 1", got)
	}
	if got := testutil.ToFloat64(backupFailureTotal.WithLabelValues(cluster.Namespace, cluster.Name)); got != 1 {
		t.Fatalf("backupFailureTotal after dedupe = %v, want 1", got)
	}
}

func TestApplyBackupJobSnapshotToMetrics(t *testing.T) {
	successJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0))},
		Status: batchv1.JobStatus{
			CompletionTime: ptrToTime(time.Unix(1700000100, 0)),
		},
	}
	failureJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0))},
		Status: batchv1.JobStatus{
			CompletionTime: ptrToTime(time.Unix(1700000200, 0)),
		},
	}

	tests := []struct {
		name            string
		cluster         *openbaov1alpha1.OpenBaoCluster
		snapshot        backupJobMetricsSnapshot
		wantState       float64
		wantAttemptUnix float64
		wantInProgress  float64
	}{
		{
			name:           "in progress takes priority",
			cluster:        newTestClusterWithBackup("metrics-progress", "backup-ns"),
			snapshot:       backupJobMetricsSnapshot{inProgress: true, newestSucceeded: successJob, newestFailed: failureJob},
			wantState:      3,
			wantInProgress: 1,
		},
		{
			name:            "latest completed failure wins",
			cluster:         newTestClusterWithBackup("metrics-failure", "backup-ns"),
			snapshot:        backupJobMetricsSnapshot{newestSucceeded: successJob, newestFailed: failureJob},
			wantState:       2,
			wantAttemptUnix: 1700000200,
			wantInProgress:  0,
		},
		{
			name: "status fallback uses last backup time",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newTestClusterWithBackup("metrics-status-success", "backup-ns")
				lastBackup := metav1.NewTime(time.Unix(1700000300, 0))
				cluster.Status.Backup = &openbaov1alpha1.BackupStatus{LastBackupTime: &lastBackup}
				return cluster
			}(),
			snapshot:       backupJobMetricsSnapshot{},
			wantState:      1,
			wantInProgress: 0,
		},
		{
			name: "status fallback uses consecutive failures",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newTestClusterWithBackup("metrics-status-failure", "backup-ns")
				cluster.Status.Backup = &openbaov1alpha1.BackupStatus{ConsecutiveFailures: 2}
				return cluster
			}(),
			snapshot:       backupJobMetricsSnapshot{},
			wantState:      2,
			wantInProgress: 0,
		},
		{
			name:           "no data resets to none",
			cluster:        newTestClusterWithBackup("metrics-none", "backup-ns"),
			snapshot:       backupJobMetricsSnapshot{},
			wantState:      0,
			wantInProgress: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := NewMetrics(tt.cluster.Namespace, tt.cluster.Name)
			resetBackupTestState(tt.cluster.Namespace, tt.cluster.Name)
			defer metrics.Clear()

			newBackupManager(newTestClient(t)).applyBackupJobSnapshotToMetrics(tt.cluster, metrics, tt.snapshot)

			if got := testutil.ToFloat64(backupState.WithLabelValues(tt.cluster.Namespace, tt.cluster.Name)); got != tt.wantState {
				t.Fatalf("backupState = %v, want %v", got, tt.wantState)
			}
			if got := testutil.ToFloat64(backupInProgress.WithLabelValues(tt.cluster.Namespace, tt.cluster.Name)); got != tt.wantInProgress {
				t.Fatalf("backupInProgress = %v, want %v", got, tt.wantInProgress)
			}
			if tt.wantAttemptUnix != 0 {
				if got := testutil.ToFloat64(backupLastAttemptTimestamp.WithLabelValues(tt.cluster.Namespace, tt.cluster.Name)); got != tt.wantAttemptUnix {
					t.Fatalf("backupLastAttemptTimestamp = %v, want %v", got, tt.wantAttemptUnix)
				}
			}
		})
	}
}

func TestSyncBackupMetricsAndHelperSelection(t *testing.T) {
	t.Run("nil metrics is a no-op", func(t *testing.T) {
		cluster := newTestClusterWithBackup("sync-noop", "backup-ns")
		if err := newBackupManager(newTestClient(t)).syncBackupMetrics(context.Background(), logr.Discard(), cluster, nil); err != nil {
			t.Fatalf("syncBackupMetrics() error = %v, want nil", err)
		}
	})

	t.Run("list error is wrapped", func(t *testing.T) {
		cluster := newTestClusterWithBackup("sync-error", "backup-ns")
		client := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return errors.New("list failed")
				},
			}).
			Build()

		err := newBackupManager(client).syncBackupMetrics(context.Background(), logr.Discard(), cluster, NewMetrics(cluster.Namespace, cluster.Name))
		if err == nil || err.Error() != "failed to list backup jobs for metrics sync: list failed" {
			t.Fatalf("syncBackupMetrics() error = %v, want wrapped list failure", err)
		}
	})

	t.Run("newestBackupJob uses completion time when available", func(t *testing.T) {
		older := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0))},
			Status: batchv1.JobStatus{
				CompletionTime: ptrToTime(time.Unix(1700000050, 0)),
			},
		}
		newerByCreation := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Unix(1700000100, 0))},
		}
		newerByCompletion := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(time.Unix(1699999900, 0))},
			Status: batchv1.JobStatus{
				CompletionTime: ptrToTime(time.Unix(1700000200, 0)),
			},
		}

		if got := newestBackupJob(nil, older); got != older {
			t.Fatal("newestBackupJob(nil, older) did not return older")
		}
		if got := newestBackupJob(older, nil); got != older {
			t.Fatal("newestBackupJob(older, nil) did not return older")
		}
		if got := newestBackupJob(older, newerByCreation); got != newerByCreation {
			t.Fatal("newestBackupJob() did not prefer newer creation timestamp")
		}
		if got := newestBackupJob(newerByCreation, newerByCompletion); got != newerByCompletion {
			t.Fatal("newestBackupJob() did not prefer newer completion timestamp")
		}
		if got := backupJobTimestamp(newerByCompletion); !got.Equal(time.Unix(1700000200, 0)) {
			t.Fatalf("backupJobTimestamp() = %v, want completion time", got)
		}
		if got := backupJobTimestamp(newerByCreation); !got.Equal(time.Unix(1700000100, 0)) {
			t.Fatalf("backupJobTimestamp() = %v, want creation time", got)
		}
		if got := backupJobTimestamp(nil); !got.IsZero() {
			t.Fatalf("backupJobTimestamp(nil) = %v, want zero", got)
		}
	})
}

func TestHandleManualTrigger(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()

	t.Run("returns manual trigger when no active job exists", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-backup", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: "now"}

		manager := newBackupManager(newTestClient(t, cluster))
		manual, scheduledTime, err := manager.handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err != nil {
			t.Fatalf("handleManualTrigger() error = %v", err)
		}
		if !manual {
			t.Fatal("manual = false, want true")
		}
		if !scheduledTime.Equal(now) {
			t.Fatalf("scheduledTime = %v, want %v", scheduledTime, now)
		}
		if cluster.Annotations[constants.AnnotationTriggerBackup] != "now" {
			t.Fatal("manual trigger annotation was cleared unexpectedly")
		}
	})

	t.Run("ignores empty trigger annotation", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-empty", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: ""}

		manager := newBackupManager(newTestClient(t, cluster))
		manual, scheduledTime, err := manager.handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err != nil {
			t.Fatalf("handleManualTrigger() error = %v", err)
		}
		if manual {
			t.Fatal("manual = true, want false")
		}
		if !scheduledTime.IsZero() {
			t.Fatalf("scheduledTime = %v, want zero", scheduledTime)
		}
	})

	t.Run("clears trigger when backup job already active", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-active", "backup-ns")
		cluster.Annotations = map[string]string{
			constants.AnnotationTriggerBackup: "now",
			"openbao.org/keep":                "preserved",
		}
		job := newBackupJobForCluster(cluster, "backup-running", now)
		job.Status.Active = 1

		client := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(cluster, job).
			Build()

		manager := newBackupManager(client)
		manual, scheduledTime, err := manager.handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err != nil {
			t.Fatalf("handleManualTrigger() error = %v", err)
		}
		if manual {
			t.Fatal("manual = true, want false")
		}
		if !scheduledTime.IsZero() {
			t.Fatalf("scheduledTime = %v, want zero", scheduledTime)
		}
		if _, ok := cluster.Annotations[constants.AnnotationTriggerBackup]; ok {
			t.Fatal("manual trigger annotation still present on in-memory object")
		}
		if got := cluster.Annotations["openbao.org/keep"]; got != "preserved" {
			t.Fatalf("unrelated annotation = %q, want preserved", got)
		}

		updated := &openbaov1alpha1.OpenBaoCluster{}
		if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
			t.Fatalf("Get() cluster error = %v", err)
		}
		if _, ok := updated.Annotations[constants.AnnotationTriggerBackup]; ok {
			t.Fatal("manual trigger annotation still present on persisted object")
		}
		if got := updated.Annotations["openbao.org/keep"]; got != "preserved" {
			t.Fatalf("persisted unrelated annotation = %q, want preserved", got)
		}
	})

	t.Run("returns list error", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-error", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: "now"}

		client := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(cluster).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return errors.New("list failed")
				},
			}).
			Build()

		_, _, err := newBackupManager(client).handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err == nil || err.Error() != "failed to check for active backup job: failed to list backup jobs: list failed" {
			t.Fatalf("handleManualTrigger() error = %v, want wrapped list failure", err)
		}
	})
}

func TestClearTriggerAnnotationLogsPatchError(t *testing.T) {
	cluster := newTestClusterWithBackup("manual-patch-error", "backup-ns")
	cluster.Annotations = map[string]string{
		constants.AnnotationTriggerBackup: "now",
		"openbao.org/keep":                "preserved",
	}

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("patch failed")
			},
		}).
		Build()

	manager := newBackupManager(client)
	logSink := &capturingLogSink{}
	logger := logr.New(logSink)

	manager.clearTriggerAnnotation(context.Background(), logger, cluster, constants.AnnotationTriggerBackup)

	if logSink.errorCount != 1 {
		t.Fatalf("error log count = %d, want 1", logSink.errorCount)
	}
	if logSink.infoCount != 0 {
		t.Fatalf("info log count = %d, want 0", logSink.infoCount)
	}
	if _, ok := cluster.Annotations[constants.AnnotationTriggerBackup]; ok {
		t.Fatal("manual trigger annotation still present on in-memory object after patch failure")
	}
	if got := cluster.Annotations["openbao.org/keep"]; got != "preserved" {
		t.Fatalf("in-memory unrelated annotation = %q, want preserved", got)
	}

	persisted := &openbaov1alpha1.OpenBaoCluster{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, persisted); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if _, ok := persisted.Annotations[constants.AnnotationTriggerBackup]; !ok {
		t.Fatal("persisted manual trigger annotation removed despite patch failure")
	}
	if got := persisted.Annotations["openbao.org/keep"]; got != "preserved" {
		t.Fatalf("persisted unrelated annotation = %q, want preserved", got)
	}
}

func TestRecordBackupAttemptPersistsStatus(t *testing.T) {
	cluster := newTestClusterWithBackup("record-attempt", "backup-ns")
	cluster.Status.Backup = nil

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithStatusSubresource(cluster).
		WithObjects(cluster).
		Build()

	manager := newBackupManager(client)
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
	if err := client.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if updated.Status.Backup == nil || updated.Status.Backup.LastAttemptTime == nil || !updated.Status.Backup.LastAttemptTime.Time.Equal(now) {
		t.Fatalf("persisted LastAttemptTime = %#v, want %v", updated.Status.Backup, now)
	}
}

func TestJobDurationAndBackupJobKeyAndReadBackupSize(t *testing.T) {
	t.Run("jobDuration handles missing and invalid timestamps", func(t *testing.T) {
		start := metav1.NewTime(time.Unix(1700000000, 0))
		end := metav1.NewTime(time.Unix(1700000060, 0))

		tests := []struct {
			name   string
			job    *batchv1.Job
			want   time.Duration
			wantOK bool
		}{
			{name: "nil job"},
			{name: "missing start", job: &batchv1.Job{Status: batchv1.JobStatus{CompletionTime: &end}}},
			{name: "missing completion", job: &batchv1.Job{Status: batchv1.JobStatus{StartTime: &start}}},
			{name: "zero duration", job: &batchv1.Job{Status: batchv1.JobStatus{StartTime: &start, CompletionTime: &start}}},
			{name: "negative duration", job: &batchv1.Job{Status: batchv1.JobStatus{StartTime: &end, CompletionTime: &start}}},
			{name: "valid duration", job: &batchv1.Job{Status: batchv1.JobStatus{StartTime: &start, CompletionTime: &end}}, want: time.Minute, wantOK: true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, ok := jobDuration(tt.job)
				if got != tt.want || ok != tt.wantOK {
					t.Fatalf("jobDuration() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.wantOK)
				}
			})
		}
	})

	t.Run("backupJobKey returns annotated key", func(t *testing.T) {
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"openbao.org/backup-key": "backups/key.snap"}}}
		if got := backupJobKey(job); got != "backups/key.snap" {
			t.Fatalf("backupJobKey() = %q, want %q", got, "backups/key.snap")
		}
		if got := backupJobKey(&batchv1.Job{}); got != "" {
			t.Fatalf("backupJobKey() without annotations = %q, want empty", got)
		}
	})

	t.Run("shouldReadBackupSizeFromObjectStorage requires static credentials", func(t *testing.T) {
		cluster := newTestClusterWithBackup("size-policy", "backup-ns")
		cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "backup-creds"}
		if !shouldReadBackupSizeFromObjectStorage(cluster) {
			t.Fatal("shouldReadBackupSizeFromObjectStorage() = false, want true")
		}

		cluster.Spec.Backup.Target.RoleARN = "arn:aws:iam::123456789012:role/backup"
		if shouldReadBackupSizeFromObjectStorage(cluster) {
			t.Fatal("shouldReadBackupSizeFromObjectStorage() = true with RoleARN, want false")
		}
	})

	t.Run("readBackupSizeFromObjectStorage handles storage outcomes", func(t *testing.T) {
		cluster := newTestClusterWithBackup("size-read", "backup-ns")
		manager := newBackupManager(newTestClient(t))
		originalOpenBlobStoreFn := openBlobStoreFn
		defer func() { openBlobStoreFn = originalOpenBlobStoreFn }()

		tests := []struct {
			name          string
			cluster       *openbaov1alpha1.OpenBaoCluster
			backupKey     string
			openFn        func(context.Context, storage.Config) (blobstore.BlobStore, error)
			wantSize      int64
			wantErr       string
			wantOpenCalls int
			wantClose     int
		}{
			{
				name:      "nil cluster short circuits",
				cluster:   nil,
				backupKey: "backups/key.snap",
			},
			{
				name:      "empty key short circuits",
				cluster:   cluster,
				backupKey: "",
			},
			{
				name:      "open error is wrapped",
				cluster:   cluster,
				backupKey: "backups/key.snap",
				openFn: func(context.Context, storage.Config) (blobstore.BlobStore, error) {
					return nil, errors.New("open failed")
				},
				wantErr:       "failed to create storage client: open failed",
				wantOpenCalls: 1,
			},
			{
				name:      "head error is wrapped and store closes",
				cluster:   cluster,
				backupKey: "backups/key.snap",
				openFn: func(context.Context, storage.Config) (blobstore.BlobStore, error) {
					return &metricsBlobStore{headErr: errors.New("head failed")}, nil
				},
				wantErr:       "failed to head object \"backups/key.snap\": head failed",
				wantOpenCalls: 1,
				wantClose:     1,
			},
			{
				name:      "nil head info returns zero size",
				cluster:   cluster,
				backupKey: "backups/key.snap",
				openFn: func(context.Context, storage.Config) (blobstore.BlobStore, error) {
					return &metricsBlobStore{}, nil
				},
				wantOpenCalls: 1,
				wantClose:     1,
			},
			{
				name:      "head info returns size",
				cluster:   cluster,
				backupKey: "backups/key.snap",
				openFn: func(context.Context, storage.Config) (blobstore.BlobStore, error) {
					return &metricsBlobStore{headInfo: &blobstore.ObjectInfo{Size: 8192}}, nil
				},
				wantSize:      8192,
				wantOpenCalls: 1,
				wantClose:     1,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				openCalls := 0
				var store *metricsBlobStore

				openBlobStoreFn = func(ctx context.Context, cfg storage.Config) (blobstore.BlobStore, error) {
					openCalls++
					if tt.openFn == nil {
						return nil, nil
					}
					openedStore, err := tt.openFn(ctx, cfg)
					if metricsStore, ok := openedStore.(*metricsBlobStore); ok {
						store = metricsStore
					}
					return openedStore, err
				}

				size, err := manager.readBackupSizeFromObjectStorage(context.Background(), tt.cluster, tt.backupKey)
				if tt.wantErr == "" {
					if err != nil {
						t.Fatalf("readBackupSizeFromObjectStorage() error = %v, want nil", err)
					}
				} else if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("readBackupSizeFromObjectStorage() error = %v, want %q", err, tt.wantErr)
				}
				if size != tt.wantSize {
					t.Fatalf("readBackupSizeFromObjectStorage() size = %d, want %d", size, tt.wantSize)
				}
				if openCalls != tt.wantOpenCalls {
					t.Fatalf("openBlobStoreFn call count = %d, want %d", openCalls, tt.wantOpenCalls)
				}
				gotClose := 0
				if store != nil {
					gotClose = store.closeCount
				}
				if gotClose != tt.wantClose {
					t.Fatalf("blob store Close() call count = %d, want %d", gotClose, tt.wantClose)
				}
			})
		}
	})
}

func TestBackfillBackupGaugesFromLatestSuccess(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	start := ptrToTime(now)
	completion := ptrToTime(now.Add(2 * time.Minute))
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "backup-success",
			Namespace:   "backup-ns",
			Annotations: map[string]string{"openbao.org/backup-key": "backups/key.snap"},
		},
		Status: batchv1.JobStatus{
			StartTime:      start,
			CompletionTime: completion,
			Succeeded:      1,
		},
	}

	t.Run("backfills duration and size into metrics and status", func(t *testing.T) {
		cluster := newTestClusterWithBackup("backfill-success", "backup-ns")
		cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "backup-creds"}
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: cluster.Namespace},
		}
		client := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithStatusSubresource(cluster).
			WithObjects(cluster, secret).
			Build()

		manager := newBackupManager(client)
		metrics := NewMetrics(cluster.Namespace, cluster.Name)
		resetBackupTestState(cluster.Namespace, cluster.Name)
		defer metrics.Clear()

		store := &metricsBlobStore{headInfo: &blobstore.ObjectInfo{Size: 4096}}
		originalOpenBlobStoreFn := openBlobStoreFn
		openBlobStoreFn = func(context.Context, storage.Config) (blobstore.BlobStore, error) {
			return store, nil
		}
		defer func() { openBlobStoreFn = originalOpenBlobStoreFn }()

		if err := manager.backfillBackupGaugesFromLatestSuccess(context.Background(), logr.Discard(), cluster, metrics, job); err != nil {
			t.Fatalf("backfillBackupGaugesFromLatestSuccess() error = %v", err)
		}

		if got := testutil.ToFloat64(backupLastSuccessTimestamp.WithLabelValues(cluster.Namespace, cluster.Name)); got != float64(completion.Unix()) {
			t.Fatalf("backupLastSuccessTimestamp = %v, want %v", got, float64(completion.Unix()))
		}
		if got := testutil.ToFloat64(backupLastDurationSeconds.WithLabelValues(cluster.Namespace, cluster.Name)); got != 120 {
			t.Fatalf("backupLastDurationSeconds = %v, want 120", got)
		}
		if got := testutil.ToFloat64(backupLastSizeBytes.WithLabelValues(cluster.Namespace, cluster.Name)); got != 4096 {
			t.Fatalf("backupLastSizeBytes = %v, want 4096", got)
		}
		if cluster.Status.Backup.LastBackupDuration != "2m0s" {
			t.Fatalf("LastBackupDuration = %q, want %q", cluster.Status.Backup.LastBackupDuration, "2m0s")
		}
		if cluster.Status.Backup.LastBackupSize != 4096 {
			t.Fatalf("LastBackupSize = %d, want 4096", cluster.Status.Backup.LastBackupSize)
		}
		if store.closeCount != 1 {
			t.Fatalf("blob store closeCount = %d, want 1", store.closeCount)
		}
	})

	t.Run("patch failure is wrapped when backfill changes status", func(t *testing.T) {
		cluster := newTestClusterWithBackup("backfill-error", "backup-ns")
		cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "backup-creds"}
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: cluster.Namespace},
		}

		client := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithStatusSubresource(cluster).
			WithObjects(cluster, secret).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceApply: func(context.Context, client.Client, string, runtime.ApplyConfiguration, ...client.SubResourceApplyOption) error {
					return errors.New("apply failed")
				},
			}).
			Build()

		manager := newBackupManager(client)
		metrics := NewMetrics(cluster.Namespace, cluster.Name)
		resetBackupTestState(cluster.Namespace, cluster.Name)
		defer metrics.Clear()

		originalOpenBlobStoreFn := openBlobStoreFn
		openBlobStoreFn = func(context.Context, storage.Config) (blobstore.BlobStore, error) {
			return &metricsBlobStore{headInfo: &blobstore.ObjectInfo{Size: 1024}}, nil
		}
		defer func() { openBlobStoreFn = originalOpenBlobStoreFn }()

		err := manager.backfillBackupGaugesFromLatestSuccess(context.Background(), logr.Discard(), cluster, metrics, job)
		if err == nil || err.Error() != "failed to patch backup status after metrics backfill: apply failed" {
			t.Fatalf("backfillBackupGaugesFromLatestSuccess() error = %v, want wrapped apply failure", err)
		}
	})
}

func ptrToTime(value time.Time) *metav1.Time {
	metaTime := metav1.NewTime(value)
	return &metaTime
}
