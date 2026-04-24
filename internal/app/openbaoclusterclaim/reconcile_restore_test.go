package openbaoclusterclaim

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestResolveActiveRestoreRequestReturnsEarliestNonTerminalRequest(t *testing.T) {
	t.Parallel()

	clusterRef := &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao-a1b2c3"}
	createdAt := time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC)

	terminal := newRestoreFixture("restore-terminal", openbaov1alpha1.RestorePhaseCompleted, "Restore completed")
	terminal.Spec.Cluster = clusterRef.Name
	terminal.CreationTimestamp = metav1.NewTime(createdAt.Add(-2 * time.Minute))

	activeOlder := newRestoreFixture("restore-1", openbaov1alpha1.RestorePhaseRunning, "Restore job is running")
	activeOlder.Spec.Cluster = clusterRef.Name
	activeOlder.CreationTimestamp = metav1.NewTime(createdAt)

	activeNewer := newRestoreFixture("restore-2", openbaov1alpha1.RestorePhaseValidating, "Validating restore preconditions")
	activeNewer.Spec.Cluster = clusterRef.Name
	activeNewer.CreationTimestamp = metav1.NewTime(createdAt.Add(time.Minute))

	otherCluster := newRestoreFixture("restore-other", openbaov1alpha1.RestorePhaseRunning, "Restore job is running")
	otherCluster.Spec.Cluster = "other-cluster"

	_, builder := newClaimTestClientBuilder(t)
	c := builder.WithObjects(terminal, activeOlder, activeNewer, otherCluster).Build()
	reconciler := runtimeReconciler{reader: c}

	restore, err := reconciler.resolveActiveRestoreExecution(context.Background(), clusterRef, result{Valid: true})
	if err != nil {
		t.Fatalf("resolveActiveRestoreExecution() error = %v", err)
	}
	if restore == nil {
		t.Fatal("resolveActiveRestoreExecution() = nil, want earliest non-terminal restore")
	}
	if restore.Name != activeOlder.Name {
		t.Fatalf("resolveActiveRestoreExecution() name = %q, want %q", restore.Name, activeOlder.Name)
	}
}

func TestResolveActiveClaimRestoreRequestReturnsEarliestNonTerminalRequest(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	createdAt := time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC)

	terminal := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         claim.Namespace,
			Name:              "restore-terminal",
			CreationTimestamp: metav1.NewTime(createdAt.Add(-2 * time.Minute)),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
		},
	}
	activeOlder := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         claim.Namespace,
			Name:              "restore-1",
			CreationTimestamp: metav1.NewTime(createdAt),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning,
		},
	}
	activeNewer := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         claim.Namespace,
			Name:              "restore-2",
			CreationTimestamp: metav1.NewTime(createdAt.Add(time.Minute)),
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending,
		},
	}
	otherClaim := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      "restore-other",
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "other-claim"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning,
		},
	}

	_, builder := newClaimTestClientBuilder(t, claim)
	c := builder.WithObjects(claim.DeepCopy(), terminal, activeOlder, activeNewer, otherClaim).Build()
	reconciler := runtimeReconciler{reader: c}

	request, err := reconciler.resolveActiveRestoreRequest(context.Background(), claim)
	if err != nil {
		t.Fatalf("resolveActiveRestoreRequest() error = %v", err)
	}
	if request == nil {
		t.Fatal("resolveActiveRestoreRequest() = nil, want earliest non-terminal request")
	}
	if request.Name != activeOlder.Name {
		t.Fatalf("resolveActiveRestoreRequest() name = %q, want %q", request.Name, activeOlder.Name)
	}
}

func TestDesiredRestoreStatusDefaultsUnreconciledExecutionToPending(t *testing.T) {
	t.Parallel()

	restore := newRestoreFixture("restore-1", "", "")
	restore.Status.StartTime = nil
	restore.Status.SnapshotKey = ""
	restore.Spec.Source.Key = "snapshots/backup-1.snap"

	status := desiredRestoreStatus(nil, restore)
	if status == nil {
		t.Fatal("desiredRestoreStatus() = nil, want restore summary")
	}
	if status.ExecutionRef == nil || status.ExecutionRef.Name != restore.Name {
		t.Fatalf("desiredRestoreStatus() executionRef = %#v, want %q", status.ExecutionRef, restore.Name)
	}
	if status.State != openbaov1alpha1.RestorePhasePending {
		t.Fatalf("desiredRestoreStatus() state = %q, want %q", status.State, openbaov1alpha1.RestorePhasePending)
	}
	if status.SnapshotKey != "snapshots/backup-1.snap" {
		t.Fatalf("desiredRestoreStatus() snapshotKey = %q, want %q", status.SnapshotKey, "snapshots/backup-1.snap")
	}
}

func TestReconcileClaimPublishesActiveRestoreSummary(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization = sameClusterMaterializationStatus()

	localCluster := validClaimManagedLocalCluster()
	localCluster.Name = claim.Status.Materialization.LocalRef.Name
	localCluster.Status = openbaov1alpha1.OpenBaoClusterStatus{Phase: openbaov1alpha1.ClusterPhaseRunning}
	restore := newRestoreFixture("restore-1", openbaov1alpha1.RestorePhaseRunning, "Restore Job openbao-restore-restore-1 is running; waiting for completion.")
	restore.Spec.Cluster = localCluster.Name

	restoreRequest := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      "restore-request-1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State:      openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning,
			Reason:     "Validating",
			RestoreRef: &openbaov1alpha1.NamespacedReference{Namespace: restore.Namespace, Name: "restore-1"},
		},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+7)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
		restoreRequest,
		restore,
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Restore == nil {
		t.Fatal("claim status restore = nil, want active restore workflow summary")
	}
	if updated.Status.Restore.RequestRef == nil || updated.Status.Restore.RequestRef.Name != restoreRequest.Name {
		t.Fatalf("claim status restore requestRef = %#v, want %q", updated.Status.Restore.RequestRef, restoreRequest.Name)
	}
	if updated.Status.Restore.ExecutionRef == nil || updated.Status.Restore.ExecutionRef.Namespace != restore.Namespace || updated.Status.Restore.ExecutionRef.Name != restore.Name {
		t.Fatalf("claim status restore executionRef = %#v, want %q", updated.Status.Restore.ExecutionRef, restore.Name)
	}
	if updated.Status.Restore.RequestState != openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning {
		t.Fatalf("claim status restore requestState = %q, want %q", updated.Status.Restore.RequestState, openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning)
	}
	if updated.Status.Restore.State != openbaov1alpha1.RestorePhaseRunning {
		t.Fatalf("claim status restore state = %q, want %q", updated.Status.Restore.State, openbaov1alpha1.RestorePhaseRunning)
	}
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded {
		t.Fatalf("claim status phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded)
	}
	if updated.Status.Summary == nil {
		t.Fatal("claim summary = nil, want active restore summary")
	}
	if updated.Status.Summary.Severity != openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning {
		t.Fatalf("claim summary severity = %q, want %q", updated.Status.Summary.Severity, openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning)
	}
	if updated.Status.Summary.Reason != string(openbaov1alpha1.RestorePhaseRunning) {
		t.Fatalf("claim summary reason = %q, want %q", updated.Status.Summary.Reason, openbaov1alpha1.RestorePhaseRunning)
	}
	if updated.Status.Summary.Message != restore.Status.Message {
		t.Fatalf("claim summary message = %q, want %q", updated.Status.Summary.Message, restore.Status.Message)
	}
	if updated.Status.Summary.SourceRef == nil || updated.Status.Summary.SourceRef.Kind != "OpenBaoClusterClaimRestoreRequest" || updated.Status.Summary.SourceRef.Name != restoreRequest.Name {
		t.Fatalf("claim summary sourceRef = %#v, want active claim restore request", updated.Status.Summary.SourceRef)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceAvailable, metav1.ConditionTrue, string(openbaov1alpha1.RestorePhaseRunning))
	assertCondition(t, updated.Status.Conditions, conditionTypeMaintenanceActive, metav1.ConditionTrue, string(openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning))
}

func TestReconcileClaimOmitsTerminalRestoreSummary(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization = sameClusterMaterializationStatus()

	localCluster := validClaimManagedLocalCluster()
	localCluster.Name = claim.Status.Materialization.LocalRef.Name
	localCluster.Status = openbaov1alpha1.OpenBaoClusterStatus{Phase: openbaov1alpha1.ClusterPhaseRunning}

	restoreRequest := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      "restore-request-1",
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: claim.Name},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateSucceeded,
		},
	}
	restore := newRestoreFixture("restore-1", openbaov1alpha1.RestorePhaseCompleted, "Restore completed successfully")
	restore.Spec.Cluster = localCluster.Name

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+7)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
		restoreRequest,
		restore,
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Restore != nil {
		t.Fatalf("claim status restore = %#v, want nil once restore is terminal", updated.Status.Restore)
	}
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseReady {
		t.Fatalf("claim status phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseReady)
	}
}

func newRestoreFixture(name string, phase openbaov1alpha1.RestorePhase, message string) *openbaov1alpha1.OpenBaoRestore {
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "payments-bao",
			Source: openbaov1alpha1.RestoreSource{
				Key: "snapshots/backup.snap",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: "https://backup.example.internal",
					Bucket:   "backups",
				},
			},
			Force: true,
		},
		Status: openbaov1alpha1.OpenBaoRestoreStatus{
			Phase:       phase,
			Message:     message,
			StartTime:   &metav1.Time{Time: time.Date(2026, time.April, 23, 11, 0, 0, 0, time.UTC)},
			SnapshotKey: "snapshots/backup.snap",
		},
	}
	return restore
}
