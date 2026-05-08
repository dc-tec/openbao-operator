package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestHandleManualTrigger(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()

	t.Run("returns manual trigger when no active job exists", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-backup", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: "now"}

		manager := newBackupManager(newTestClient(t, cluster))
		triggerToken, scheduledTime, err := manager.handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err != nil {
			t.Fatalf("handleManualTrigger() error = %v", err)
		}
		if triggerToken != "now" {
			t.Fatalf("triggerToken = %q, want now", triggerToken)
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
		triggerToken, scheduledTime, err := manager.handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err != nil {
			t.Fatalf("handleManualTrigger() error = %v", err)
		}
		if triggerToken != "" {
			t.Fatalf("triggerToken = %q, want empty", triggerToken)
		}
		if !scheduledTime.IsZero() {
			t.Fatalf("scheduledTime = %v, want zero", scheduledTime)
		}
	})

	t.Run("clears trigger when backup job already active", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-active", "backup-ns")
		cluster.Annotations = map[string]string{
			constants.AnnotationTriggerBackup: "now",
			testKeepAnnotationKey:             testKeepAnnotationValue,
		}
		job := newBackupJobForCluster(cluster, "backup-running", now)
		job.Status.Active = 1

		k8sClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(cluster, job).
			Build()

		manager := newBackupManager(k8sClient)
		triggerToken, scheduledTime, err := manager.handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err != nil {
			t.Fatalf("handleManualTrigger() error = %v", err)
		}
		if triggerToken != "" {
			t.Fatalf("triggerToken = %q, want empty", triggerToken)
		}
		if !scheduledTime.IsZero() {
			t.Fatalf("scheduledTime = %v, want zero", scheduledTime)
		}
		if _, ok := cluster.Annotations[constants.AnnotationTriggerBackup]; ok {
			t.Fatal("manual trigger annotation still present on in-memory object")
		}
		if got := cluster.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
			t.Fatalf("unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
		}

		updated := &openbaov1alpha1.OpenBaoCluster{}
		if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, updated); err != nil {
			t.Fatalf("Get() cluster error = %v", err)
		}
		if _, ok := updated.Annotations[constants.AnnotationTriggerBackup]; ok {
			t.Fatal("manual trigger annotation still present on persisted object")
		}
		if got := updated.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
			t.Fatalf("persisted unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
		}
	})

	t.Run("returns list error", func(t *testing.T) {
		cluster := newTestClusterWithBackup("manual-error", "backup-ns")
		cluster.Annotations = map[string]string{constants.AnnotationTriggerBackup: "now"}

		k8sClient := fake.NewClientBuilder().
			WithScheme(testScheme).
			WithObjects(cluster).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
					return errors.New("list failed")
				},
			}).
			Build()

		_, _, err := newBackupManager(k8sClient).handleManualTrigger(context.Background(), logr.Discard(), cluster, now)
		if err == nil || err.Error() != "failed to check for active backup job: failed to list backup jobs: list failed" {
			t.Fatalf("handleManualTrigger() error = %v, want wrapped list failure", err)
		}
	})
}

func TestClearTriggerAnnotationLogsPatchError(t *testing.T) {
	cluster := newTestClusterWithBackup("manual-patch-error", "backup-ns")
	cluster.Annotations = map[string]string{
		constants.AnnotationTriggerBackup: "now",
		testKeepAnnotationKey:             testKeepAnnotationValue,
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(cluster).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
				return errors.New("patch failed")
			},
		}).
		Build()

	manager := newBackupManager(k8sClient)
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
	if got := cluster.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
		t.Fatalf("in-memory unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
	}

	persisted := &openbaov1alpha1.OpenBaoCluster{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, persisted); err != nil {
		t.Fatalf("Get() cluster error = %v", err)
	}
	if _, ok := persisted.Annotations[constants.AnnotationTriggerBackup]; !ok {
		t.Fatal("persisted manual trigger annotation removed despite patch failure")
	}
	if got := persisted.Annotations[testKeepAnnotationKey]; got != testKeepAnnotationValue {
		t.Fatalf("persisted unrelated annotation = %q, want %s", got, testKeepAnnotationValue)
	}
}
