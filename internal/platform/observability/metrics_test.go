package observability

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestReconcileMetrics_NoPanic(t *testing.T) {
	m := NewReconcileMetrics("ns", "name", "ctrl")

	// These calls should not panic and will register/update metrics for the
	// given label set.
	m.ObserveDuration(0.5)
	m.ObserveDuration(1.0)
	m.IncrementError("Error")
}

func TestClusterMetrics_NoPanic(t *testing.T) {
	m := NewClusterMetrics("ns", "name")

	m.SetReadyReplicas(3)
	m.SetReadReplicaCounts(2, 2, 2, 2)
	m.SetPhase(openbaov1alpha1.ClusterPhaseInitializing)
	m.SetPhase(openbaov1alpha1.ClusterPhaseRunning)
	m.Clear()
}

func TestReconcileMetrics_EmitsSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())
	controller := fmt.Sprintf("ctrl-%s", t.Name())

	durationBefore := testutil.CollectAndCount(reconcileDurationHistogram)
	errorsBefore := testutil.CollectAndCount(reconcileErrorsTotal)

	m := NewReconcileMetrics(namespace, name, controller)
	m.ObserveDuration(0.1)
	m.IncrementError("TestError")

	durationAfter := testutil.CollectAndCount(reconcileDurationHistogram)
	errorsAfter := testutil.CollectAndCount(reconcileErrorsTotal)

	if durationAfter != durationBefore+1 {
		t.Fatalf("expected reconcile duration series to increase by 1 (before=%d, after=%d)", durationBefore, durationAfter)
	}
	if errorsAfter != errorsBefore+1 {
		t.Fatalf("expected reconcile error series to increase by 1 (before=%d, after=%d)", errorsBefore, errorsAfter)
	}
}

func TestRestoreMetrics_EmitsSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())

	totalBefore := testutil.CollectAndCount(restoreTotal)
	successBefore := testutil.CollectAndCount(restoreSuccessTotal)
	failureBefore := testutil.CollectAndCount(restoreFailureTotal)
	durationBefore := testutil.CollectAndCount(restoreDurationHistogram)

	m := NewRestoreMetrics(namespace, name)
	m.RecordStarted()
	m.RecordSuccess(1.0)
	m.RecordFailureWithDuration(2.0)

	totalAfter := testutil.CollectAndCount(restoreTotal)
	successAfter := testutil.CollectAndCount(restoreSuccessTotal)
	failureAfter := testutil.CollectAndCount(restoreFailureTotal)
	durationAfter := testutil.CollectAndCount(restoreDurationHistogram)

	if totalAfter != totalBefore+1 {
		t.Fatalf("expected restore_total series to increase by 1 (before=%d, after=%d)", totalBefore, totalAfter)
	}
	if successAfter != successBefore+1 {
		t.Fatalf("expected restore_success_total series to increase by 1 (before=%d, after=%d)", successBefore, successAfter)
	}
	if failureAfter != failureBefore+1 {
		t.Fatalf("expected restore_failure_total series to increase by 1 (before=%d, after=%d)", failureBefore, failureAfter)
	}
	// RecordSuccess + RecordFailureWithDuration should create exactly one histogram series.
	if durationAfter != durationBefore+1 {
		t.Fatalf("expected restore duration histogram series to increase by 1 (before=%d, after=%d)", durationBefore, durationAfter)
	}
}

func TestClusterMetrics_ReadReplicaSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())

	desiredBefore := testutil.CollectAndCount(clusterReadReplicasDesiredGauge)
	readyBefore := testutil.CollectAndCount(clusterReadReplicasReadyGauge)
	registeredBefore := testutil.CollectAndCount(clusterReadReplicasRegisteredGauge)
	healthyBefore := testutil.CollectAndCount(clusterReadReplicasHealthyGauge)

	m := NewClusterMetrics(namespace, name)
	m.SetReadReplicaCounts(2, 2, 2, 1)

	desiredAfter := testutil.CollectAndCount(clusterReadReplicasDesiredGauge)
	readyAfter := testutil.CollectAndCount(clusterReadReplicasReadyGauge)
	registeredAfter := testutil.CollectAndCount(clusterReadReplicasRegisteredGauge)
	healthyAfter := testutil.CollectAndCount(clusterReadReplicasHealthyGauge)

	if desiredAfter != desiredBefore+1 {
		t.Fatalf("expected read replica desired series to increase by 1 (before=%d, after=%d)", desiredBefore, desiredAfter)
	}
	if readyAfter != readyBefore+1 {
		t.Fatalf("expected read replica ready series to increase by 1 (before=%d, after=%d)", readyBefore, readyAfter)
	}
	if registeredAfter != registeredBefore+1 {
		t.Fatalf("expected read replica registered series to increase by 1 (before=%d, after=%d)", registeredBefore, registeredAfter)
	}
	if healthyAfter != healthyBefore+1 {
		t.Fatalf("expected read replica healthy series to increase by 1 (before=%d, after=%d)", healthyBefore, healthyAfter)
	}

	m.Clear()
}

func TestClaimMetrics_ReplaceAndClearDynamicSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("claim-%s", t.Name())

	summaryBefore := testutil.CollectAndCount(claimSummaryGauge)
	infoBefore := testutil.CollectAndCount(claimInfoGauge)
	conditionBefore := testutil.CollectAndCount(claimConditionGauge)
	restoreBefore := testutil.CollectAndCount(claimRestoreStateGauge)

	claim := &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "tenant-a"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "profile-a"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimStatus{
			Phase: openbaov1alpha1.OpenBaoClusterClaimPhaseDegraded,
			Rollout: openbaov1alpha1.OpenBaoClusterClaimRolloutStatus{
				State: openbaov1alpha1.OpenBaoClusterClaimRolloutStateRollingOut,
			},
			Materialization: openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
				Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			},
			Summary: &openbaov1alpha1.OpenBaoClusterClaimStatusSummary{
				Severity: openbaov1alpha1.OpenBaoClusterClaimStatusSeverityWarning,
				Reason:   "RollingOut",
			},
			Restore: &openbaov1alpha1.OpenBaoClusterClaimRestoreStatus{
				RequestRef: &openbaov1alpha1.LocalReference{Name: "restore-a"},
				State:      openbaov1alpha1.RestorePhaseRunning,
			},
			Conditions: []metav1.Condition{
				{Type: "ServiceAvailable", Status: metav1.ConditionTrue},
			},
		},
	}

	SyncClaim(claim)

	if after := testutil.CollectAndCount(claimSummaryGauge); after != summaryBefore+1 {
		t.Fatalf("expected claim summary series to increase by 1 (before=%d, after=%d)", summaryBefore, after)
	}
	if after := testutil.CollectAndCount(claimInfoGauge); after != infoBefore+1 {
		t.Fatalf("expected claim info series to increase by 1 (before=%d, after=%d)", infoBefore, after)
	}
	if after := testutil.CollectAndCount(claimConditionGauge); after != conditionBefore+1 {
		t.Fatalf("expected claim condition series to increase by 1 (before=%d, after=%d)", conditionBefore, after)
	}
	if after := testutil.CollectAndCount(claimRestoreStateGauge); after != restoreBefore+1 {
		t.Fatalf("expected claim restore state series to increase by 1 (before=%d, after=%d)", restoreBefore, after)
	}

	claim.Status.Summary.Reason = "BackupRunning"
	claim.Status.Restore.RequestRef.Name = "restore-b"
	claim.Status.Conditions[0].Status = metav1.ConditionFalse
	SyncClaim(claim)

	if after := testutil.CollectAndCount(claimSummaryGauge); after != summaryBefore+1 {
		t.Fatalf("expected claim summary series count to stay stable after replacement (before=%d, after=%d)", summaryBefore, after)
	}
	if after := testutil.CollectAndCount(claimConditionGauge); after != conditionBefore+1 {
		t.Fatalf("expected claim condition series count to stay stable after replacement (before=%d, after=%d)", conditionBefore, after)
	}
	if after := testutil.CollectAndCount(claimRestoreStateGauge); after != restoreBefore+1 {
		t.Fatalf("expected claim restore series count to stay stable after replacement (before=%d, after=%d)", restoreBefore, after)
	}

	ClearClaim(namespace, name)

	if after := testutil.CollectAndCount(claimSummaryGauge); after != summaryBefore {
		t.Fatalf("expected claim summary series to clear back to baseline (before=%d, after=%d)", summaryBefore, after)
	}
	if after := testutil.CollectAndCount(claimInfoGauge); after != infoBefore {
		t.Fatalf("expected claim info series to clear back to baseline (before=%d, after=%d)", infoBefore, after)
	}
	if after := testutil.CollectAndCount(claimConditionGauge); after != conditionBefore {
		t.Fatalf("expected claim condition series to clear back to baseline (before=%d, after=%d)", conditionBefore, after)
	}
	if after := testutil.CollectAndCount(claimRestoreStateGauge); after != restoreBefore {
		t.Fatalf("expected claim restore series to clear back to baseline (before=%d, after=%d)", restoreBefore, after)
	}
}

func TestClaimUpgradeRequestMetrics_ReplaceAndClear(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("upgrade-%s", t.Name())

	stateBefore := testutil.CollectAndCount(claimUpgradeRequestStateGauge)
	classBefore := testutil.CollectAndCount(claimUpgradeRequestClassificationGauge)

	request := &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "claim-a"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatus{
			State:  openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStatePending,
			Reason: "WaitingForClaim",
			Classification: &openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestClassificationStatus{
				Class: openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassInPlace,
			},
		},
	}

	SyncClaimUpgradeRequest(request)

	if after := testutil.CollectAndCount(claimUpgradeRequestStateGauge); after != stateBefore+1 {
		t.Fatalf("expected upgrade request state series to increase by 1 (before=%d, after=%d)", stateBefore, after)
	}
	if after := testutil.CollectAndCount(claimUpgradeRequestClassificationGauge); after != classBefore+1 {
		t.Fatalf("expected upgrade request classification series to increase by 1 (before=%d, after=%d)", classBefore, after)
	}

	request.Status.State = openbaov1alpha1.OpenBaoClusterClaimUpgradeRequestStateRollingOut
	request.Status.Reason = "RollingOut"
	request.Status.Classification.Class = openbaov1alpha1.OpenBaoClusterClaimUpgradeClassificationClassBlocked
	SyncClaimUpgradeRequest(request)

	if after := testutil.CollectAndCount(claimUpgradeRequestStateGauge); after != stateBefore+1 {
		t.Fatalf("expected upgrade request state series count to stay stable after replacement (before=%d, after=%d)", stateBefore, after)
	}
	if after := testutil.CollectAndCount(claimUpgradeRequestClassificationGauge); after != classBefore+1 {
		t.Fatalf("expected upgrade request classification series count to stay stable after replacement (before=%d, after=%d)", classBefore, after)
	}

	ClearClaimUpgradeRequest(namespace, name)

	if after := testutil.CollectAndCount(claimUpgradeRequestStateGauge); after != stateBefore {
		t.Fatalf("expected upgrade request state series to clear back to baseline (before=%d, after=%d)", stateBefore, after)
	}
	if after := testutil.CollectAndCount(claimUpgradeRequestClassificationGauge); after != classBefore {
		t.Fatalf("expected upgrade request classification series to clear back to baseline (before=%d, after=%d)", classBefore, after)
	}
}

func TestClaimBackupRequestMetrics_ReplaceAndClear(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("backup-%s", t.Name())

	stateBefore := testutil.CollectAndCount(claimBackupRequestStateGauge)

	request := &openbaov1alpha1.OpenBaoClusterClaimBackupRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimBackupRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "claim-a"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatus{
			State:  openbaov1alpha1.OpenBaoClusterClaimBackupRequestStatePending,
			Reason: "WaitingForCluster",
		},
	}

	assertWorkflowRequestMetricLifecycle(
		t,
		"backup",
		stateBefore,
		func() {
			SyncClaimBackupRequest(request)
		},
		func() {
			request.Status.State = openbaov1alpha1.OpenBaoClusterClaimBackupRequestStateRunning
			request.Status.Reason = "Running"
			SyncClaimBackupRequest(request)
		},
		func() {
			ClearClaimBackupRequest(namespace, name)
		},
		func() int {
			return testutil.CollectAndCount(claimBackupRequestStateGauge)
		},
	)
}

func TestClaimRestoreRequestMetrics_ReplaceAndClear(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("restore-%s", t.Name())

	stateBefore := testutil.CollectAndCount(claimRestoreRequestStateGauge)

	request := &openbaov1alpha1.OpenBaoClusterClaimRestoreRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestSpec{
			ClaimRef: openbaov1alpha1.LocalReference{Name: "claim-a"},
		},
		Status: openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatus{
			State:  openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStatePending,
			Reason: "WaitingForCluster",
		},
	}

	assertWorkflowRequestMetricLifecycle(
		t,
		"restore",
		stateBefore,
		func() {
			SyncClaimRestoreRequest(request)
		},
		func() {
			request.Status.State = openbaov1alpha1.OpenBaoClusterClaimRestoreRequestStateRunning
			request.Status.Reason = "Running"
			SyncClaimRestoreRequest(request)
		},
		func() {
			ClearClaimRestoreRequest(namespace, name)
		},
		func() int {
			return testutil.CollectAndCount(claimRestoreRequestStateGauge)
		},
	)
}

func assertWorkflowRequestMetricLifecycle(
	t *testing.T,
	kind string,
	before int,
	syncInitial func(),
	syncUpdated func(),
	clear func(),
	collect func() int,
) {
	t.Helper()

	syncInitial()
	if after := collect(); after != before+1 {
		t.Fatalf("expected %s request state series to increase by 1 (before=%d, after=%d)", kind, before, after)
	}

	syncUpdated()
	if after := collect(); after != before+1 {
		t.Fatalf("expected %s request state series count to stay stable after replacement (before=%d, after=%d)", kind, before, after)
	}

	clear()
	if after := collect(); after != before {
		t.Fatalf("expected %s request state series to clear back to baseline (before=%d, after=%d)", kind, before, after)
	}
}
