package openbaoclusterclaimbackuprequest

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	testBackupClaimName        = "payments-bao"
	testBackupClusterNamespace = "tenant-payments"
	testBackupFailedReason     = "BackupFailed"
)

func TestReconcileRequestState_ServiceClaimsDisabled(t *testing.T) {
	t.Parallel()

	reconciler := runtimeReconciler{enableServiceClaims: false}
	state, reason, clusterRef, startTime, completionTime, snapshotKey := reconciler.reconcileRequestState(context.Background(), &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{})
	if state != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked)
	}
	if reason != "ServiceClaimsDisabled" {
		t.Fatalf("reason = %q, want ServiceClaimsDisabled", reason)
	}
	if clusterRef != nil || startTime != nil || completionTime != nil || snapshotKey != "" {
		t.Fatalf("unexpected non-empty state payload: %#v %#v %#v %q", clusterRef, startTime, completionTime, snapshotKey)
	}
}

func TestReconcileRequestState_PreservesTerminalStatus(t *testing.T) {
	t.Parallel()

	start := metav1.NewTime(time.Unix(1700000000, 0).UTC())
	completed := metav1.NewTime(time.Unix(1700000060, 0).UTC())
	request := newBackupRequest("backup-terminal")
	request.Status.State = openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed
	request.Status.Reason = testBackupFailedReason
	request.Status.ClusterRef = &openbaov1alpha1.NamespacedReference{Namespace: testBackupClusterNamespace, Name: testBackupClaimName}
	request.Status.StartTime = &start
	request.Status.CompletionTime = &completed
	request.Status.SnapshotKey = "snapshots/payments-bao/failed.snap"

	reconciler := runtimeReconciler{enableServiceClaims: true}
	state, reason, clusterRef, startTime, completionTime, snapshotKey := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed)
	}
	if reason != testBackupFailedReason {
		t.Fatalf("reason = %q, want %s", reason, testBackupFailedReason)
	}
	if clusterRef == nil || clusterRef.Namespace != testBackupClusterNamespace || clusterRef.Name != testBackupClaimName {
		t.Fatalf("clusterRef = %#v, want tenant-payments/payments-bao", clusterRef)
	}
	if startTime == nil || completionTime == nil {
		t.Fatalf("start/completion = %#v %#v, want both set", startTime, completionTime)
	}
	if snapshotKey != "snapshots/payments-bao/failed.snap" {
		t.Fatalf("snapshotKey = %q, want preserved snapshot", snapshotKey)
	}
}

func TestReconcileRequestState_RequestsManualBackup(t *testing.T) {
	t.Parallel()

	reconciler := newBackupRequestTestReconciler(t, baseBackupRequestObjects()...)
	request := newBackupRequest("backup-1")

	state, reason, clusterRef, startTime, completionTime, snapshotKey := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending)
	}
	if reason != "BackupRequested" {
		t.Fatalf("reason = %q, want BackupRequested", reason)
	}
	if clusterRef == nil || clusterRef.Namespace != testBackupClusterNamespace || clusterRef.Name != testBackupClaimName {
		t.Fatalf("clusterRef = %#v, want tenant-payments/payments-bao", clusterRef)
	}
	if startTime != nil || completionTime != nil || snapshotKey != "" {
		t.Fatalf("unexpected start/completion/snapshot = %#v %#v %q", startTime, completionTime, snapshotKey)
	}

	var cluster openbaov1alpha1.OpenBaoCluster
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: testBackupClusterNamespace, Name: testBackupClaimName}, &cluster); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if got := cluster.Annotations[constants.AnnotationTriggerBackup]; got != string(request.UID) {
		t.Fatalf("trigger annotation = %q, want %q", got, string(request.UID))
	}
}

func TestReconcileRequestState_SucceedsAfterBackupCompletes(t *testing.T) {
	t.Parallel()

	objects := baseBackupRequestObjects()
	for _, obj := range objects {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok {
			continue
		}
		start := metav1.NewTime(time.Unix(1700000000, 0).UTC())
		complete := metav1.NewTime(time.Unix(1700000060, 0).UTC())
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{
			LastHandledManualTrigger: "backup-uid-1",
			LastAttemptTime:          &start,
			LastBackupTime:           &complete,
			LastBackupName:           "snapshots/payments-bao/backup-1.snap",
		}
	}
	reconciler := newBackupRequestTestReconciler(t, objects...)
	request := newBackupRequest("backup-1")

	state, reason, clusterRef, startTime, completionTime, snapshotKey := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded)
	}
	if reason != reasonBackupCompleted {
		t.Fatalf("reason = %q, want %q", reason, reasonBackupCompleted)
	}
	if clusterRef == nil || clusterRef.Name != testBackupClaimName {
		t.Fatalf("clusterRef = %#v, want payments-bao", clusterRef)
	}
	if startTime == nil || completionTime == nil {
		t.Fatalf("start/completion = %#v %#v, want both set", startTime, completionTime)
	}
	if snapshotKey != "snapshots/payments-bao/backup-1.snap" {
		t.Fatalf("snapshotKey = %q, want propagated backup key", snapshotKey)
	}
}

func TestReconcileRequestState_FailsAfterBackupFailure(t *testing.T) {
	t.Parallel()

	objects := baseBackupRequestObjects()
	for _, obj := range objects {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok {
			continue
		}
		start := metav1.NewTime(time.Unix(1700000000, 0).UTC())
		failed := metav1.NewTime(time.Unix(1700000060, 0).UTC())
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
		cluster.Status.Backup = &openbaov1alpha1.BackupStatus{
			LastHandledManualTrigger: "backup-uid-1",
			LastAttemptTime:          &start,
			LastFailureTime:          &failed,
			LastFailureReason:        testBackupFailedReason,
			LastFailureMessage:       "job failed",
		}
	}
	reconciler := newBackupRequestTestReconciler(t, objects...)
	request := newBackupRequest("backup-1")

	state, reason, _, startTime, completionTime, snapshotKey := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateFailed)
	}
	if reason != testBackupFailedReason {
		t.Fatalf("reason = %q, want %s", reason, testBackupFailedReason)
	}
	if startTime == nil || completionTime == nil {
		t.Fatalf("start/completion = %#v %#v, want both set", startTime, completionTime)
	}
	if snapshotKey != "" {
		t.Fatalf("snapshotKey = %q, want empty", snapshotKey)
	}
}

func TestReconcileRequestState_BlocksWhenAnotherRequestIsActive(t *testing.T) {
	t.Parallel()

	reconciler := newBackupRequestTestReconciler(t, append(baseBackupRequestObjects(), &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "payments",
			Name:              "backup-1",
			UID:               types.UID("backup-uid-older"),
			CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0).UTC()),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: testBackupClaimName},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending,
		},
	})...)
	request := newBackupRequest("backup-2")
	request.CreationTimestamp = metav1.NewTime(time.Unix(1700000100, 0).UTC())

	state, reason, _, _, _, _ := reconciler.reconcileRequestState(context.Background(), request)
	if state != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateBlocked)
	}
	if reason != "AnotherBackupRequestActive" {
		t.Fatalf("reason = %q, want AnotherBackupRequestActive", reason)
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}

func newBackupRequestTestReconciler(t *testing.T, objects ...client.Object) runtimeReconciler {
	t.Helper()

	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoClusterClaimBackupRequest{}).
		WithObjects(objects...).
		Build()
	return runtimeReconciler{
		client:              fakeClient,
		reader:              fakeClient,
		enableServiceClaims: true,
	}
}

func newBackupRequest(name string) *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
	return &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "payments",
			Name:              name,
			UID:               types.UID("backup-uid-1"),
			CreationTimestamp: metav1.NewTime(time.Unix(1700000001, 0).UTC()),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: testBackupClaimName},
		},
	}
}

func baseBackupRequestObjects() []client.Object {
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      testBackupClaimName,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:          openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef:  openbaov1alpha1.LocalReference{Name: "standard-v1"},
			ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Materialization: openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
				Mode:     openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
				LocalRef: &openbaov1alpha1.NamespacedReference{Namespace: testBackupClusterNamespace, Name: testBackupClaimName},
			},
		},
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   testBackupClusterNamespace,
			Name:        testBackupClaimName,
			Annotations: map[string]string{},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase: openbaov1alpha1.ClusterPhaseRunning,
		},
	}
	return []client.Object{claim, cluster}
}
