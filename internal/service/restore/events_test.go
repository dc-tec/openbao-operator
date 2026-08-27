package restore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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

func expectAnyEventContains(t *testing.T, recorder *events.FakeRecorder, attempts int, parts ...string) {
	t.Helper()

	for i := 0; i < attempts; i++ {
		select {
		case event := <-recorder.Events:
			match := true
			for _, part := range parts {
				if !strings.Contains(event, part) {
					match = false
					break
				}
			}
			if match {
				return
			}
		case <-time.After(time.Second):
			t.Fatal("expected event, got none")
		}
	}

	t.Fatalf("did not find event containing %q within %d attempts", strings.Join(parts, ", "), attempts)
}

func newRestoreEventScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))
	return scheme
}

func newRestoreEventCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			UID:       "test-cluster-uid",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}
}

func newRestoreEventResource() *openbaov1alpha1.OpenBaoRestore {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-restore",
			Namespace:       "default",
			UID:             "test-restore-uid",
			ResourceVersion: "1",
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "test-cluster",
			Image:   "openbao/restore:latest",
			Source: openbaov1alpha1.RestoreSource{
				Key: "backup-key",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://s3.amazonaws.com",
					Bucket:   "test-bucket",
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhasePending,
		},
	}
	return restore
}

func TestHandlePending_EmitsValidationStartedEvent(t *testing.T) {
	t.Parallel()

	scheme := newRestoreEventScheme(t)
	restore := newRestoreEventResource()
	recorder := events.NewFakeRecorder(10)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
		WithReturnManagedFields().
		Build()
	mgr := NewManager(k8sClient, scheme, recorder, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	result, err := mgr.handlePending(context.Background(), testLogger(), restore)
	if err != nil {
		t.Fatalf("handlePending() error = %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("result = %+v, want positive requeue", result)
	}

	expectEventContains(t, recorder, "Normal", ReasonRestoreValidationStarted)
}

func TestCreateRestoreJob_EmitsJobCreatedEvent(t *testing.T) {
	t.Parallel()

	scheme := newRestoreEventScheme(t)
	cluster := newRestoreEventCluster()
	restore := newRestoreEventResource()
	restore.Status.Phase = openbaov1alpha1.RestorePhaseRunning
	restore.Status.Execution = newRestoreExecutionStatus(restore)
	recorder := events.NewFakeRecorder(10)
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, restore).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
		WithReturnManagedFields().
		Build()
	mgr := NewManager(k8sClient, scheme, recorder, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

	result, err := mgr.createRestoreJob(context.Background(), testLogger(), restore, cluster, restoreJobName(restore))
	if err != nil {
		t.Fatalf("createRestoreJob() error = %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("result = %+v, want positive requeue", result)
	}

	expectAnyEventContains(t, recorder, 2, "Normal", ReasonRestoreIdentityConfiguration)
	expectAnyEventContains(t, recorder, 2, "Normal", ReasonRestoreJobCreated)
}

func TestRestoreTerminalEvents(t *testing.T) {
	t.Parallel()

	t.Run("failed", func(t *testing.T) {
		t.Parallel()

		scheme := newRestoreEventScheme(t)
		cluster := newRestoreEventCluster()
		restore := newRestoreEventResource()
		restore.Status.Phase = openbaov1alpha1.RestorePhaseRunning

		recorder := events.NewFakeRecorder(10)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, restore).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
			WithReturnManagedFields().
			Build()
		mgr := NewManager(k8sClient, scheme, recorder, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

		if _, err := mgr.failRestore(context.Background(), testLogger(), restore, "restore failed"); err != nil {
			t.Fatalf("failRestore() error = %v", err)
		}

		expectEventContains(t, recorder, "Warning", ReasonRestoreFailed)
	})

	t.Run("completed", func(t *testing.T) {
		t.Parallel()

		scheme := newRestoreEventScheme(t)
		cluster := newRestoreEventCluster()
		restore := newRestoreEventResource()
		restore.Status.Phase = openbaov1alpha1.RestorePhaseRunning

		recorder := events.NewFakeRecorder(10)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cluster, restore).
			WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
			WithReturnManagedFields().
			Build()
		mgr := NewManager(k8sClient, scheme, recorder, security.NewImageVerifier(testLogger(), k8sClient, nil), "")

		if err := mgr.completeRestore(context.Background(), testLogger(), restore, "restore completed"); err != nil {
			t.Fatalf("completeRestore() error = %v", err)
		}

		expectEventContains(t, recorder, "Normal", ReasonRestoreCompleted)
	})
}
