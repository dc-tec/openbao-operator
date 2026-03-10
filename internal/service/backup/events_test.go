package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func expectEventContains(t *testing.T, recorder *events.FakeRecorder, parts ...string) {
	t.Helper()

	select {
	case event := <-recorder.Events:
		for _, part := range parts {
			if !strings.Contains(event, part) {
				t.Fatalf("event %q does not contain %q", event, part)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("expected event, got none")
	}
}

func TestHandleManualTrigger_EmitsAcceptedEvent(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0).UTC()
	cluster := newTestClusterWithBackup("manual-events", "backup-ns")
	cluster.Annotations = map[string]string{"openbao.org/trigger-backup": "now"}

	recorder := events.NewFakeRecorder(10)
	k8sClient := newTestClient(t, cluster)
	manager := NewManager(
		k8sClient,
		testScheme,
		portopenbao.ClientConfig{},
		security.NewImageVerifier(logr.Discard(), k8sClient, nil),
		"",
		recorder,
	)

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

	expectEventContains(t, recorder, "Normal", ReasonBackupManualTriggerAccepted)
}

func TestProcessBackupJobResult_EmitsCompletedEvent(t *testing.T) {
	t.Parallel()

	cluster := newTestClusterWithBackup("backup-events-success", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
			Annotations: map[string]string{
				"openbao.org/backup-key": "test-key-abc",
			},
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
		},
	}

	recorder := events.NewFakeRecorder(10)
	k8sClient := newTestClient(t, job)
	manager := NewManager(
		k8sClient,
		testScheme,
		portopenbao.ClientConfig{},
		security.NewImageVerifier(logr.Discard(), k8sClient, nil),
		"",
		recorder,
	)

	statusUpdated, err := manager.processBackupJobResult(context.Background(), logr.Discard(), cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}
	if !statusUpdated {
		t.Fatal("statusUpdated = false, want true")
	}

	expectEventContains(t, recorder, "Normal", ReasonBackupCompleted)
}

func TestProcessBackupJobResult_EmitsFailedEvent(t *testing.T) {
	t.Parallel()

	cluster := newTestClusterWithBackup("backup-events-failed", "default")
	scheduled := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	jobName := backupJobName(cluster, scheduled)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: cluster.Namespace,
		},
		Status: batchv1.JobStatus{
			Failed: 1,
		},
	}

	recorder := events.NewFakeRecorder(10)
	k8sClient := newTestClient(t, job)
	manager := NewManager(
		k8sClient,
		testScheme,
		portopenbao.ClientConfig{},
		security.NewImageVerifier(logr.Discard(), k8sClient, nil),
		"",
		recorder,
	)

	statusUpdated, err := manager.processBackupJobResult(context.Background(), logr.Discard(), cluster, jobName)
	if err != nil {
		t.Fatalf("processBackupJobResult() error = %v", err)
	}
	if !statusUpdated {
		t.Fatal("statusUpdated = false, want true")
	}

	expectEventContains(t, recorder, "Warning", ReasonBackupFailed)
}
