package restorerequest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim/requestworkflow"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	testClaimNamespace       = "payments"
	testClusterNamespace     = "tenant-payments"
	testClaimName            = "payments-bao"
	testRestoreRequested     = "RestoreRequested"
	testLatestBackupSnapshot = "snapshots/payments-bao/backup-1.snap"
	testSelectedBackup       = "snapshots/payments-bao/manual-20260423.snap"
)

func TestReconcileRequestState_ServiceClaimsDisabled(t *testing.T) {
	t.Parallel()

	reconciler := runtimeReconciler{enableServiceClaims: false}
	state, reason, clusterRef, restoreRef, startTime, completionTime, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked)
	}
	if reason != "ServiceClaimsDisabled" {
		t.Fatalf("reason = %q, want ServiceClaimsDisabled", reason)
	}
	if clusterRef != nil || restoreRef != nil || startTime != nil || completionTime != nil || snapshotKey != "" {
		t.Fatalf("unexpected non-empty state payload: %#v %#v %#v %#v %q", clusterRef, restoreRef, startTime, completionTime, snapshotKey)
	}
}

func TestReconcileEmitsRestoreRequestEvent(t *testing.T) {
	t.Parallel()

	request := newRestoreRequest("restore-event")
	recorder := events.NewFakeRecorder(2)
	reconciler := newRestoreRequestTestReconciler(t, request)
	reconciler.recorder = recorder

	if _, err := reconciler.Reconcile(context.Background(), client.ObjectKeyFromObject(request), logr.Discard()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	expectEventContains(t, recorder, "Warning", requestworkflow.ReasonClaimNotFound)
}

func TestReconcileRequestState_RequestsManualRestore(t *testing.T) {
	t.Parallel()

	reconciler := newRestoreRequestTestReconciler(t, baseRestoreRequestObjects()...)
	request := newRestoreRequest("restore-1")

	state, reason, clusterRef, restoreRef, startTime, completionTime, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending)
	}
	if reason != testRestoreRequested {
		t.Fatalf("reason = %q, want %s", reason, testRestoreRequested)
	}
	if clusterRef == nil || clusterRef.Namespace != testClusterNamespace || clusterRef.Name != testClaimName {
		t.Fatalf("clusterRef = %#v, want %s/%s", clusterRef, testClusterNamespace, testClaimName)
	}
	if restoreRef == nil || restoreRef.Name != request.Name {
		t.Fatalf("restoreRef = %#v, want %q", restoreRef, request.Name)
	}
	if startTime != nil || completionTime != nil {
		t.Fatalf("unexpected start/completion = %#v %#v", startTime, completionTime)
	}
	if snapshotKey != testLatestBackupSnapshot {
		t.Fatalf("snapshotKey = %q, want propagated backup key", snapshotKey)
	}

	var restore openbaov1alpha1.OpenBaoRestore
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: testClusterNamespace, Name: request.Name}, &restore); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if restore.Spec.Cluster != testClaimName {
		t.Fatalf("restore cluster = %q, want %s", restore.Spec.Cluster, testClaimName)
	}
	if restore.Spec.Source.Key != snapshotKey {
		t.Fatalf("restore source key = %q, want %q", restore.Spec.Source.Key, snapshotKey)
	}
	if restore.Spec.Image != "example.com/openbao-backup:test" {
		t.Fatalf("restore image = %q, want backup helper image fallback", restore.Spec.Image)
	}
	if restore.Spec.TokenSecretRef != nil {
		t.Fatalf("restore tokenSecretRef = %#v, want nil for self-init OIDC cluster", restore.Spec.TokenSecretRef)
	}
	if restore.Labels[constants.LabelOpenBaoClaimRestoreRequest] != request.Name {
		t.Fatalf("restore label %q = %q, want %q", constants.LabelOpenBaoClaimRestoreRequest, restore.Labels[constants.LabelOpenBaoClaimRestoreRequest], request.Name)
	}
}

func TestReconcileRequestState_UsesRestoreHelperImage(t *testing.T) {
	t.Parallel()

	objects := baseRestoreRequestObjects()
	cluster := objects[1].(*openbaov1alpha1.OpenBaoCluster)
	cluster.Spec.Restore = &openbaov1alpha1.RestoreConfig{Image: "example.com/openbao-restore:test"}
	reconciler := newRestoreRequestTestReconciler(t, objects...)
	request := newRestoreRequest("restore-image")

	state, reason, _, _, _, _, _ := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending {
		t.Fatalf("state = %q reason = %q, want pending restore request", state, reason)
	}

	var restore openbaov1alpha1.OpenBaoRestore
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: testClusterNamespace, Name: request.Name}, &restore); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if restore.Spec.Image != "example.com/openbao-restore:test" {
		t.Fatalf("restore image = %q, want restore helper image", restore.Spec.Image)
	}
}

func TestReconcileRequestState_RestoresSelectedBackupRequest(t *testing.T) {
	t.Parallel()

	backupRequest := newSucceededBackupRequest("backup-selected", testSelectedBackup)
	reconciler := newRestoreRequestTestReconciler(t, append(baseRestoreRequestObjects(), backupRequest)...)
	request := newRestoreRequest("restore-selected")
	request.Spec.Source = &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
		Mode: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
		BackupRequestRef: &openbaov1alpha1.LocalReference{
			Name: backupRequest.Name,
		},
	}

	state, reason, _, restoreRef, _, _, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending)
	}
	if reason != testRestoreRequested {
		t.Fatalf("reason = %q, want %s", reason, testRestoreRequested)
	}
	if restoreRef == nil || restoreRef.Name != request.Name {
		t.Fatalf("restoreRef = %#v, want %q", restoreRef, request.Name)
	}
	if snapshotKey != testSelectedBackup {
		t.Fatalf("snapshotKey = %q, want selected backup key", snapshotKey)
	}

	var restore openbaov1alpha1.OpenBaoRestore
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: testClusterNamespace, Name: request.Name}, &restore); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if restore.Spec.Source.Key != testSelectedBackup {
		t.Fatalf("restore source key = %q, want %q", restore.Spec.Source.Key, testSelectedBackup)
	}
}

func TestReconcileRequestState_BlocksInvalidBackupRequestSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		source        *openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec
		backupRequest *openbaov1alpha1.OpenBaoClusterClaimBackupRequest
		wantReason    string
	}{
		{
			name: "missing backup request ref",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
			},
			wantReason: "BackupRequestRefRequired",
		},
		{
			name: "backup request not found",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-missing"},
			},
			wantReason: "BackupRequestNotFound",
		},
		{
			name: "backup request claim mismatch",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-other-claim"},
			},
			backupRequest: func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
				request := newSucceededBackupRequest("backup-other-claim", testSelectedBackup)
				request.Spec.ClaimRef.Name = "other-bao"
				return request
			}(),
			wantReason: "BackupRequestClaimMismatch",
		},
		{
			name: "backup request cluster mismatch",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-other-cluster"},
			},
			backupRequest: func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
				request := newSucceededBackupRequest("backup-other-cluster", testSelectedBackup)
				request.Status.ClusterRef = &openbaov1alpha1.NamespacedReference{Namespace: "other", Name: testClaimName}
				return request
			}(),
			wantReason: "BackupRequestClusterMismatch",
		},
		{
			name: "backup request cluster unknown",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-cluster-unknown"},
			},
			backupRequest: func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
				request := newSucceededBackupRequest("backup-cluster-unknown", testSelectedBackup)
				request.Status.ClusterRef = nil
				return request
			}(),
			wantReason: "BackupRequestClusterUnknown",
		},
		{
			name: "backup request not succeeded",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-running"},
			},
			backupRequest: func() *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
				request := newSucceededBackupRequest("backup-running", testSelectedBackup)
				request.Status.State = openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning
				return request
			}(),
			wantReason: "BackupRequestNotSucceeded",
		},
		{
			name: "backup request snapshot missing",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeBackupRequest,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-no-snapshot"},
			},
			backupRequest: newSucceededBackupRequest("backup-no-snapshot", ""),
			wantReason:    "BackupRequestSnapshotMissing",
		},
		{
			name: "latest source with backup request ref",
			source: &openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceSpec{
				Mode:             openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSourceModeLatestSuccessful,
				BackupRequestRef: &openbaov1alpha1.LocalReference{Name: "backup-selected"},
			},
			wantReason: "InvalidRestoreSource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			objects := baseRestoreRequestObjects()
			if tt.backupRequest != nil {
				objects = append(objects, tt.backupRequest)
			}
			reconciler := newRestoreRequestTestReconciler(t, objects...)
			request := newRestoreRequest("restore-selected")
			request.Spec.Source = tt.source

			state, reason, _, _, _, _, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
			if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked {
				t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked)
			}
			if reason != tt.wantReason {
				t.Fatalf("reason = %q, want %s", reason, tt.wantReason)
			}
			if snapshotKey != "" {
				t.Fatalf("snapshotKey = %q, want empty", snapshotKey)
			}
		})
	}
}

func TestReconcileRequestState_UsesDefaultBackupImageWhenClusterBackupImageUnset(t *testing.T) {
	t.Setenv(constants.EnvOperatorBackupImageRepo, "example.com/openbao-backup")
	t.Setenv(constants.EnvOperatorVersion, "dev")

	objects := baseRestoreRequestObjects()
	for _, obj := range objects {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok || cluster.Spec.Backup == nil {
			continue
		}
		cluster.Spec.Backup.Image = ""
	}

	reconciler := newRestoreRequestTestReconciler(t, objects...)
	request := newRestoreRequest("restore-default-image")

	state, reason, _, restoreRef, _, _, _ := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending)
	}
	if reason != testRestoreRequested {
		t.Fatalf("reason = %q, want %s", reason, testRestoreRequested)
	}
	if restoreRef == nil {
		t.Fatal("restoreRef = nil, want created restore execution")
	}

	var restore openbaov1alpha1.OpenBaoRestore
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: testClusterNamespace, Name: restoreRef.Name}, &restore); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if restore.Spec.Image != "example.com/openbao-backup:dev" {
		t.Fatalf("restore image = %q, want %q", restore.Spec.Image, "example.com/openbao-backup:dev")
	}
}

func TestReconcileRequestState_FallsBackToRootTokenForStandardCluster(t *testing.T) {
	t.Parallel()

	objects := baseRestoreRequestObjects()
	for _, obj := range objects {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok {
			continue
		}
		cluster.Spec.SelfInit = nil
		cluster.Spec.Restore = nil
	}
	reconciler := newRestoreRequestTestReconciler(t, objects...)
	request := newRestoreRequest("restore-1")

	state, reason, _, restoreRef, _, _, _ := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending)
	}
	if reason != testRestoreRequested {
		t.Fatalf("reason = %q, want %s", reason, testRestoreRequested)
	}
	if restoreRef == nil {
		t.Fatal("restoreRef = nil, want created restore execution")
	}

	var restore openbaov1alpha1.OpenBaoRestore
	if err := reconciler.client.Get(context.Background(), types.NamespacedName{Namespace: testClusterNamespace, Name: restoreRef.Name}, &restore); err != nil {
		t.Fatalf("get restore: %v", err)
	}
	if restore.Spec.TokenSecretRef == nil || restore.Spec.TokenSecretRef.Name != testClaimName+constants.SuffixRootToken {
		t.Fatalf("restore tokenSecretRef = %#v, want %q", restore.Spec.TokenSecretRef, testClaimName+constants.SuffixRootToken)
	}
	if restore.Spec.JWTAuthRole != "" {
		t.Fatalf("restore jwtAuthRole = %q, want empty when using root token fallback", restore.Spec.JWTAuthRole)
	}
}

func TestReconcileRequestState_SucceedsAfterRestoreCompletes(t *testing.T) {
	t.Parallel()

	objects := baseRestoreRequestObjects()
	for _, obj := range objects {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if ok {
			cluster.Status.Backup.LastBackupName = testLatestBackupSnapshot
		}
	}
	start := metav1.NewTime(time.Unix(1700000000, 0).UTC())
	completed := metav1.NewTime(time.Unix(1700000300, 0).UTC())
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testClusterNamespace,
			Name:      "restore-1",
			Labels: map[string]string{
				constants.LabelOpenBaoClaimNamespace:      testClaimNamespace,
				constants.LabelOpenBaoClaimName:           testClaimName,
				constants.LabelOpenBaoClaimRestoreRequest: "restore-1",
			},
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: testClaimName,
			Source:  openbaov1alpha1.RestoreSource{Key: testLatestBackupSnapshot},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase:          openbaov1alpha1.RestorePhaseCompleted,
			StartTime:      &start,
			CompletionTime: &completed,
			SnapshotKey:    testLatestBackupSnapshot,
		},
	}
	reconciler := newRestoreRequestTestReconciler(t, append(objects, restore)...)
	request := newRestoreRequest("restore-1")
	request.Status.RestoreRef = &openbaov1alpha1.NamespacedReference{Namespace: restore.Namespace, Name: restore.Name}

	state, reason, clusterRef, restoreRef, startTime, completionTime, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded)
	}
	if reason != reasonRestoreCompleted {
		t.Fatalf("reason = %q, want %q", reason, reasonRestoreCompleted)
	}
	if clusterRef == nil || clusterRef.Name != testClaimName {
		t.Fatalf("clusterRef = %#v, want %s", clusterRef, testClaimName)
	}
	if restoreRef == nil || restoreRef.Name != restore.Name {
		t.Fatalf("restoreRef = %#v, want %q", restoreRef, restore.Name)
	}
	if startTime == nil || completionTime == nil {
		t.Fatalf("start/completion = %#v %#v, want both set", startTime, completionTime)
	}
	if snapshotKey != restore.Status.SnapshotKey {
		t.Fatalf("snapshotKey = %q, want %q", snapshotKey, restore.Status.SnapshotKey)
	}
}

func TestReconcileRequestState_FailsAfterRestoreFailure(t *testing.T) {
	t.Parallel()

	start := metav1.NewTime(time.Unix(1700000000, 0).UTC())
	failed := metav1.NewTime(time.Unix(1700000300, 0).UTC())
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testClusterNamespace,
			Name:      "restore-1",
			Labels: map[string]string{
				constants.LabelOpenBaoClaimNamespace:      testClaimNamespace,
				constants.LabelOpenBaoClaimName:           testClaimName,
				constants.LabelOpenBaoClaimRestoreRequest: "restore-1",
			},
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: testClaimName,
			Source:  openbaov1alpha1.RestoreSource{Key: testLatestBackupSnapshot},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase:          openbaov1alpha1.RestorePhaseFailed,
			StartTime:      &start,
			CompletionTime: &failed,
			SnapshotKey:    testLatestBackupSnapshot,
			Conditions: []metav1.Condition{{
				Type:   "Ready",
				Status: metav1.ConditionFalse,
				Reason: "RestoreFailed",
			}},
		},
	}
	reconciler := newRestoreRequestTestReconciler(t, append(baseRestoreRequestObjects(), restore)...)
	request := newRestoreRequest("restore-1")
	request.Status.RestoreRef = &openbaov1alpha1.NamespacedReference{Namespace: restore.Namespace, Name: restore.Name}

	state, reason, _, _, startTime, completionTime, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateFailed)
	}
	if reason != "RestoreFailed" {
		t.Fatalf("reason = %q, want RestoreFailed", reason)
	}
	if startTime == nil || completionTime == nil {
		t.Fatalf("start/completion = %#v %#v, want both set", startTime, completionTime)
	}
	if snapshotKey != testLatestBackupSnapshot {
		t.Fatalf("snapshotKey = %q, want propagated restore snapshot key", snapshotKey)
	}
}

func TestReconcileRequestState_BlocksWhenAnotherRequestIsActive(t *testing.T) {
	t.Parallel()

	reconciler := newRestoreRequestTestReconciler(t, append(baseRestoreRequestObjects(), &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         testClaimNamespace,
			Name:              "restore-older",
			UID:               types.UID("restore-uid-older"),
			CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0).UTC()),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: testClaimName},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending,
		},
	})...)
	request := newRestoreRequest("restore-2")
	request.CreationTimestamp = metav1.NewTime(time.Unix(1700000100, 0).UTC())

	state, reason, _, _, _, _, _ := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), request))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked)
	}
	if reason != "AnotherRestoreRequestActive" {
		t.Fatalf("reason = %q, want AnotherRestoreRequestActive", reason)
	}
}

func TestReconcileRequestState_BlocksWhenNoSuccessfulBackupAvailable(t *testing.T) {
	t.Parallel()

	objects := baseRestoreRequestObjects()
	for _, obj := range objects {
		cluster, ok := obj.(*openbaov1alpha1.OpenBaoCluster)
		if !ok {
			continue
		}
		cluster.Status.Backup.LastBackupName = ""
	}
	reconciler := newRestoreRequestTestReconciler(t, objects...)

	state, reason, _, _, _, _, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), newRestoreRequest("restore-1")))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked)
	}
	if reason != "NoSuccessfulBackupAvailable" {
		t.Fatalf("reason = %q, want NoSuccessfulBackupAvailable", reason)
	}
	if snapshotKey != "" {
		t.Fatalf("snapshotKey = %q, want empty", snapshotKey)
	}
}

func TestReconcileRequestState_BlocksWhenAnotherRestoreExecutionIsActive(t *testing.T) {
	t.Parallel()

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         testClusterNamespace,
			Name:              "restore-existing",
			CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0).UTC()),
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: testClaimName,
			Source:  openbaov1alpha1.RestoreSource{Key: testLatestBackupSnapshot},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase: openbaov1alpha1.RestorePhaseRunning,
		},
	}
	reconciler := newRestoreRequestTestReconciler(t, append(baseRestoreRequestObjects(), restore)...)

	state, reason, clusterRef, _, _, _, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), newRestoreRequest("restore-1")))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateBlocked)
	}
	if reason != "AnotherRestoreExecutionActive" {
		t.Fatalf("reason = %q, want AnotherRestoreExecutionActive", reason)
	}
	if clusterRef == nil || clusterRef.Name != testClaimName {
		t.Fatalf("clusterRef = %#v, want %s", clusterRef, testClaimName)
	}
	if snapshotKey != testLatestBackupSnapshot {
		t.Fatalf("snapshotKey = %q, want backup key", snapshotKey)
	}
}

func TestReconcileRequestState_ObservesOwnedRestoreWhenStatusRefMissing(t *testing.T) {
	t.Parallel()

	start := metav1.NewTime(time.Unix(1700000000, 0).UTC())
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testClusterNamespace,
			Name:      "restore-1",
			Labels: map[string]string{
				constants.LabelOpenBaoClaimNamespace:      testClaimNamespace,
				constants.LabelOpenBaoClaimName:           testClaimName,
				constants.LabelOpenBaoClaimRestoreRequest: "restore-1",
			},
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: testClaimName,
			Source:  openbaov1alpha1.RestoreSource{Key: testLatestBackupSnapshot},
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase:       openbaov1alpha1.RestorePhaseRunning,
			StartTime:   &start,
			SnapshotKey: testLatestBackupSnapshot,
		},
	}
	reconciler := newRestoreRequestTestReconciler(t, append(baseRestoreRequestObjects(), restore)...)

	state, reason, clusterRef, restoreRef, startTime, completionTime, snapshotKey := restoreEvaluationFields(reconciler.reconcileRequestState(context.Background(), newRestoreRequest("restore-1")))
	if state != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning {
		t.Fatalf("state = %q, want %q", state, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning)
	}
	if reason != string(openbaov1alpha1.RestorePhaseRunning) {
		t.Fatalf("reason = %q, want %s", reason, openbaov1alpha1.RestorePhaseRunning)
	}
	if clusterRef == nil || clusterRef.Name != testClaimName {
		t.Fatalf("clusterRef = %#v, want %s", clusterRef, testClaimName)
	}
	if restoreRef == nil || restoreRef.Namespace != testClusterNamespace || restoreRef.Name != restore.Name {
		t.Fatalf("restoreRef = %#v, want %s/%s", restoreRef, testClusterNamespace, restore.Name)
	}
	if startTime == nil || completionTime != nil {
		t.Fatalf("start/completion = %#v %#v, want start only", startTime, completionTime)
	}
	if snapshotKey != testLatestBackupSnapshot {
		t.Fatalf("snapshotKey = %q, want %q", snapshotKey, testLatestBackupSnapshot)
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

func restoreEvaluationFields(evaluation requestEvaluation) (
	openbaov1alpha1.OpenBaoClusterClaimRestoreRequestState,
	string,
	*openbaov1alpha1.NamespacedReference,
	*openbaov1alpha1.NamespacedReference,
	*metav1.Time,
	*metav1.Time,
	string,
) {
	return evaluation.state,
		evaluation.reason,
		evaluation.clusterRef,
		evaluation.restoreRef,
		evaluation.startTime,
		evaluation.completionTime,
		evaluation.snapshotKey
}

func newRestoreRequestTestReconciler(t *testing.T, objects ...client.Object) runtimeReconciler {
	t.Helper()

	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{}).
		WithStatusSubresource(&openbaov1alpha1.OpenBaoRestore{}).
		WithObjects(objects...).
		Build()
	return runtimeReconciler{
		client:              fakeClient,
		reader:              fakeClient,
		enableServiceClaims: true,
	}
}

func newRestoreRequest(name string) *openbaov1alpha1.OpenBaoClusterClaimRestoreRequest {
	return &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         testClaimNamespace,
			Name:              name,
			UID:               types.UID("restore-uid-1"),
			CreationTimestamp: metav1.NewTime(time.Unix(1700000001, 0).UTC()),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: testClaimName},
		},
	}
}

func newSucceededBackupRequest(name string, snapshotKey string) *openbaov1alpha1.OpenBaoClusterClaimBackupRequest {
	start := metav1.NewTime(time.Unix(1700000100, 0).UTC())
	completed := metav1.NewTime(time.Unix(1700000200, 0).UTC())
	return &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testClaimNamespace,
			Name:      name,
			UID:       types.UID(name + "-uid"),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: testClaimName},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatus{
			State:          openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateSucceeded,
			Reason:         "BackupCompleted",
			ClusterRef:     &openbaov1alpha1.NamespacedReference{Namespace: testClusterNamespace, Name: testClaimName},
			StartTime:      &start,
			CompletionTime: &completed,
			SnapshotKey:    snapshotKey,
		},
	}
}

func baseRestoreRequestObjects() []client.Object {
	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testClaimNamespace,
			Name:      testClaimName,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:          openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef:  openbaov1alpha1.LocalReference{Name: "standard-v1"},
			ServiceOfferingRef: &openbaov1alpha1.LocalReference{Name: "standard"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Materialization: openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
				Mode:     openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
				LocalRef: &openbaov1alpha1.NamespacedReference{Namespace: testClusterNamespace, Name: testClaimName},
			},
		},
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testClusterNamespace,
			Name:      testClaimName,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Backup: &openbaov1alpha1.BackupSchedule{
				Schedule: "0 3 * * *",
				Image:    "example.com/openbao-backup:test",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://objectstore.example.com",
					Bucket:   "backups",
				},
			},
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled: true,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Phase: openbaov1alpha1.ClusterPhaseRunning,
			Backup: &openbaov1alpha1.BackupStatus{
				LastBackupName: testLatestBackupSnapshot,
			},
		},
	}
	return []client.Object{claim, cluster}
}

func expectEventContains(t *testing.T, recorder *events.FakeRecorder, parts ...string) {
	t.Helper()

	select {
	case event := <-recorder.Events:
		for _, part := range parts {
			if !strings.Contains(event, part) {
				t.Fatalf("event %q does not contain %q", event, part)
			}
		}
	default:
		t.Fatalf("expected event containing %q, got none", strings.Join(parts, ", "))
	}
}
