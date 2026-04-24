package openbaoclusterclaim

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestReconcileSameClusterClaimProjectsHealthyBackupStatusWithoutDegradingService(t *testing.T) {
	t.Parallel()

	lastBackupTime := metav1.NewTime(time.Date(2026, time.April, 23, 8, 0, 0, 0, time.UTC))
	nextBackupTime := metav1.NewTime(time.Date(2026, time.April, 24, 8, 0, 0, 0, time.UTC))

	claim := validClaim()
	claim.Status.Materialization = sameClusterMaterializationStatus()

	localCluster := validClaimManagedLocalCluster()
	localCluster.Status = openbaov1alpha1.OpenBaoClusterStatus{
		Phase: openbaov1alpha1.ClusterPhaseRunning,
		Backup: &openbaov1alpha1.BackupStatus{
			LastBackupTime:      &lastBackupTime,
			NextScheduledBackup: &nextBackupTime,
			LastBackupDuration:  "42s",
		},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+5)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseReady {
		t.Fatalf("phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseReady)
	}
	if updated.Status.Summary != nil {
		t.Fatalf("claim summary = %#v, want nil for healthy backup history", updated.Status.Summary)
	}
	if updated.Status.Backup == nil {
		t.Fatal("claim backup status = nil, want projected backup history")
	}
	if updated.Status.Backup.InProgress {
		t.Fatal("claim backup status inProgress = true, want false")
	}
	if updated.Status.Backup.LastBackupTime == nil || !updated.Status.Backup.LastBackupTime.Equal(&lastBackupTime) {
		t.Fatalf("claim backup lastBackupTime = %v, want %v", updated.Status.Backup.LastBackupTime, lastBackupTime)
	}
	if updated.Status.Backup.NextScheduledBackup == nil || !updated.Status.Backup.NextScheduledBackup.Equal(&nextBackupTime) {
		t.Fatalf("claim backup nextScheduledBackup = %v, want %v", updated.Status.Backup.NextScheduledBackup, nextBackupTime)
	}
	if updated.Status.Backup.LastBackupDuration != "42s" {
		t.Fatalf("claim backup lastBackupDuration = %q, want %q", updated.Status.Backup.LastBackupDuration, "42s")
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceAvailable, metav1.ConditionTrue, string(openbaov1alpha1.ReasonReady))
}

func TestReconcileSameClusterClaimSurfacesActiveBackupAsDegradedButAvailable(t *testing.T) {
	t.Parallel()

	lastAttemptTime := metav1.NewTime(time.Date(2026, time.April, 23, 9, 15, 0, 0, time.UTC))

	claim := validClaim()
	claim.Status.Materialization = sameClusterMaterializationStatus()

	localCluster := validClaimManagedLocalCluster()
	localCluster.Status = openbaov1alpha1.OpenBaoClusterStatus{
		Phase: openbaov1alpha1.ClusterPhaseBackingUp,
		Backup: &openbaov1alpha1.BackupStatus{
			LastAttemptTime: &lastAttemptTime,
		},
		Conditions: []metav1.Condition{{
			Type:   string(openbaov1alpha1.ConditionBackingUp),
			Status: metav1.ConditionTrue,
			Reason: "InProgress",
		}},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+5)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded {
		t.Fatalf("phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded)
	}
	if updated.Status.Backup == nil {
		t.Fatal("claim backup status = nil, want projected active backup state")
	}
	if !updated.Status.Backup.InProgress {
		t.Fatal("claim backup status inProgress = false, want true")
	}
	if updated.Status.Backup.LastAttemptTime == nil || !updated.Status.Backup.LastAttemptTime.Equal(&lastAttemptTime) {
		t.Fatalf("claim backup lastAttemptTime = %v, want %v", updated.Status.Backup.LastAttemptTime, lastAttemptTime)
	}
	if updated.Status.Summary == nil {
		t.Fatal("claim summary = nil, want active backup summary")
	}
	if updated.Status.Summary.Severity != openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo {
		t.Fatalf("claim summary severity = %q, want %q", updated.Status.Summary.Severity, openbaov1alpha1.OpenBaoClusterClaimStatusSeverityInfo)
	}
	if updated.Status.Summary.Reason != string(openbaov1alpha1.ClusterPhaseBackingUp) {
		t.Fatalf("claim summary reason = %q, want %q", updated.Status.Summary.Reason, openbaov1alpha1.ClusterPhaseBackingUp)
	}
	if updated.Status.Summary.SourceRef == nil || updated.Status.Summary.SourceRef.Kind != "OpenBaoCluster" || updated.Status.Summary.SourceRef.Name != localCluster.Name {
		t.Fatalf("claim summary sourceRef = %#v, want local cluster", updated.Status.Summary.SourceRef)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceAvailable, metav1.ConditionTrue, string(openbaov1alpha1.ClusterPhaseBackingUp))
	assertCondition(t, updated.Status.Conditions, conditionTypeMaintenanceActive, metav1.ConditionFalse, reasonIdle)
}

func TestReconcileSameClusterClaimSurfacesBackupFailuresAsDiagnosticWarning(t *testing.T) {
	t.Parallel()

	lastAttemptTime := metav1.NewTime(time.Date(2026, time.April, 23, 9, 30, 0, 0, time.UTC))
	nextBackupTime := metav1.NewTime(time.Date(2026, time.April, 24, 9, 30, 0, 0, time.UTC))

	claim := validClaim()
	claim.Status.Materialization = sameClusterMaterializationStatus()

	localCluster := validClaimManagedLocalCluster()
	localCluster.Status = openbaov1alpha1.OpenBaoClusterStatus{
		Phase: openbaov1alpha1.ClusterPhaseRunning,
		Backup: &openbaov1alpha1.BackupStatus{
			LastAttemptTime:     &lastAttemptTime,
			NextScheduledBackup: &nextBackupTime,
			ConsecutiveFailures: 3,
			LastFailureReason:   "BackupScheduleFailed",
			LastFailureMessage:  "Scheduled backup job failed to upload the archive.",
			LastBackupDuration:  "38s",
		},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+5)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded {
		t.Fatalf("phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded)
	}
	if updated.Status.Backup == nil {
		t.Fatal("claim backup status = nil, want projected backup failure state")
	}
	if updated.Status.Backup.ConsecutiveFailures != 3 {
		t.Fatalf("claim backup consecutiveFailures = %d, want %d", updated.Status.Backup.ConsecutiveFailures, 3)
	}
	if updated.Status.Backup.LastFailureReason != "BackupScheduleFailed" {
		t.Fatalf("claim backup lastFailureReason = %q, want %q", updated.Status.Backup.LastFailureReason, "BackupScheduleFailed")
	}
	if updated.Status.Summary == nil {
		t.Fatal("claim summary = nil, want backup failure summary")
	}
	if updated.Status.Summary.Severity != openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning {
		t.Fatalf("claim summary severity = %q, want %q", updated.Status.Summary.Severity, openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning)
	}
	if updated.Status.Summary.Reason != "BackupScheduleFailed" {
		t.Fatalf("claim summary reason = %q, want %q", updated.Status.Summary.Reason, "BackupScheduleFailed")
	}
	if updated.Status.Summary.Message != "Scheduled backup job failed to upload the archive." {
		t.Fatalf("claim summary message = %q, want propagated backup failure", updated.Status.Summary.Message)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceAvailable, metav1.ConditionTrue, "BackupScheduleFailed")
}

func validClaimManagedLocalCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-bao",
			Namespace: "payments",
			Labels: map[string]string{
				constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
				constants.LabelOpenBaoClaimNamespace: "payments",
				constants.LabelOpenBaoClaimName:      "payments-bao",
			},
		},
		Spec: validExistingSameClusterConcreteSpec(),
	}
}

func sameClusterMaterializationStatus() openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus {
	return openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
		Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		LocalRef: &openbaov1alpha1.NamespacedReference{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	}
}

func TestDesiredBackupStatusOmitsEmptyState(t *testing.T) {
	t.Parallel()

	if got := desiredBackupStatus(validClaimManagedLocalCluster()); got != nil {
		t.Fatalf("desiredBackupStatus() = %#v, want nil for empty backup state", got)
	}
}

func TestDesiredBackupStatusRecognizesBackingUpCondition(t *testing.T) {
	t.Parallel()

	cluster := validClaimManagedLocalCluster()
	cluster.Status.Conditions = []metav1.Condition{{
		Type:   string(openbaov1alpha1.ConditionBackingUp),
		Status: metav1.ConditionTrue,
	}}

	got := desiredBackupStatus(cluster)
	if got == nil {
		t.Fatal("desiredBackupStatus() = nil, want projected backup status")
	}
	if !got.InProgress {
		t.Fatal("desiredBackupStatus().InProgress = false, want true")
	}
}

func TestDesiredBackupStatusProjectsActiveBackupRequest(t *testing.T) {
	t.Parallel()

	request := &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "payments-bao-backup-1",
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatus{
			State:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending,
			Reason: "BackupRequested",
		},
	}

	got := desiredBackupStatusWithRequest(nil, request)
	if got == nil {
		t.Fatal("desiredBackupStatusWithRequest() = nil, want active request projection")
	}
	if got.RequestRef == nil || got.RequestRef.Name != request.Name {
		t.Fatalf("requestRef = %#v, want %s", got.RequestRef, request.Name)
	}
	if got.RequestState != openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending {
		t.Fatalf("requestState = %q, want %q", got.RequestState, openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending)
	}
	if got.RequestReason != "BackupRequested" {
		t.Fatalf("requestReason = %q, want BackupRequested", got.RequestReason)
	}
}

func TestReconcileSameClusterClaimDoesNotOverwritePublishedConnectionDuringBackupDiagnostics(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization = sameClusterMaterializationStatus()

	localCluster := validClaimManagedLocalCluster()
	localCluster.Status = openbaov1alpha1.OpenBaoClusterStatus{
		Phase: openbaov1alpha1.ClusterPhaseBackingUp,
		Backup: &openbaov1alpha1.BackupStatus{
			ConsecutiveFailures: 1,
			LastFailureReason:   "BackupScheduleFailed",
			LastFailureMessage:  "Scheduled backup job failed to upload the archive.",
		},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+5)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	connectionSecret := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: claim.Namespace, Name: claim.Name + "-connection"}, connectionSecret); err != nil {
		t.Fatalf("get claim connection Secret error = %v", err)
	}
	if updated.Status.Connection.Endpoint == "" {
		t.Fatal("claim connection endpoint = empty, want published endpoint during backup diagnostics")
	}
}
