package upgrade

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUpgradeMetrics_EmitsSeries(t *testing.T) {
	namespace := fmt.Sprintf("ns-%s", t.Name())
	name := fmt.Sprintf("name-%s", t.Name())

	m := NewMetrics(namespace, name)

	statusBefore := testutil.CollectAndCount(upgradeStatusGauge)
	m.SetStatus(UpgradeStatusRunning)
	statusAfter := testutil.CollectAndCount(upgradeStatusGauge)
	if statusAfter != statusBefore+1 {
		t.Fatalf("expected upgrade status series to increase by 1 (before=%d, after=%d)", statusBefore, statusAfter)
	}

	inProgressBefore := testutil.CollectAndCount(upgradeInProgressGauge)
	m.SetInProgress(true)
	inProgressAfter := testutil.CollectAndCount(upgradeInProgressGauge)
	if inProgressAfter != inProgressBefore+1 {
		t.Fatalf("expected upgrade in_progress series to increase by 1 (before=%d, after=%d)", inProgressBefore, inProgressAfter)
	}

	podsCompletedBefore := testutil.CollectAndCount(upgradePodsCompletedGauge)
	m.SetPodsCompleted(1)
	podsCompletedAfter := testutil.CollectAndCount(upgradePodsCompletedGauge)
	if podsCompletedAfter != podsCompletedBefore+1 {
		t.Fatalf("expected upgrade pods_completed series to increase by 1 (before=%d, after=%d)", podsCompletedBefore, podsCompletedAfter)
	}

	podsTotalBefore := testutil.CollectAndCount(upgradeTotalPodsGauge)
	m.SetTotalPods(3)
	podsTotalAfter := testutil.CollectAndCount(upgradeTotalPodsGauge)
	if podsTotalAfter != podsTotalBefore+1 {
		t.Fatalf("expected upgrade pods_total series to increase by 1 (before=%d, after=%d)", podsTotalBefore, podsTotalAfter)
	}

	partitionBefore := testutil.CollectAndCount(upgradePartitionGauge)
	m.SetPartition(2)
	partitionAfter := testutil.CollectAndCount(upgradePartitionGauge)
	if partitionAfter != partitionBefore+1 {
		t.Fatalf("expected upgrade partition series to increase by 1 (before=%d, after=%d)", partitionBefore, partitionAfter)
	}

	stepDownBefore := testutil.CollectAndCount(upgradeStepDownCounter)
	stepDownFailuresBefore := testutil.CollectAndCount(upgradeStepDownFailuresCounter)
	m.IncrementStepDownTotal()
	m.IncrementStepDownFailures()
	stepDownAfter := testutil.CollectAndCount(upgradeStepDownCounter)
	stepDownFailuresAfter := testutil.CollectAndCount(upgradeStepDownFailuresCounter)
	if stepDownAfter != stepDownBefore+1 {
		t.Fatalf("expected upgrade stepdown_total series to increase by 1 (before=%d, after=%d)", stepDownBefore, stepDownAfter)
	}
	if stepDownFailuresAfter != stepDownFailuresBefore+1 {
		t.Fatalf("expected upgrade stepdown_failures_total series to increase by 1 (before=%d, after=%d)", stepDownFailuresBefore, stepDownFailuresAfter)
	}

	totalBefore := testutil.CollectAndCount(upgradeTotalCounter)
	successBefore := testutil.CollectAndCount(upgradeSuccessTotalCounter)
	failureBefore := testutil.CollectAndCount(upgradeFailureTotalCounter)
	rollbackBefore := testutil.CollectAndCount(upgradeRollbackTotalCounter)
	m.IncrementTotal("RollingUpdate")
	m.IncrementSuccess("RollingUpdate")
	m.IncrementFailure("RollingUpdate")
	m.IncrementRollback("RollingUpdate")
	totalAfter := testutil.CollectAndCount(upgradeTotalCounter)
	successAfter := testutil.CollectAndCount(upgradeSuccessTotalCounter)
	failureAfter := testutil.CollectAndCount(upgradeFailureTotalCounter)
	rollbackAfter := testutil.CollectAndCount(upgradeRollbackTotalCounter)
	if totalAfter != totalBefore+1 {
		t.Fatalf("expected upgrade total series to increase by 1 (before=%d, after=%d)", totalBefore, totalAfter)
	}
	if successAfter != successBefore+1 {
		t.Fatalf("expected upgrade success_total series to increase by 1 (before=%d, after=%d)", successBefore, successAfter)
	}
	if failureAfter != failureBefore+1 {
		t.Fatalf("expected upgrade failure_total series to increase by 1 (before=%d, after=%d)", failureBefore, failureAfter)
	}
	if rollbackAfter != rollbackBefore+1 {
		t.Fatalf("expected upgrade rollback_total series to increase by 1 (before=%d, after=%d)", rollbackBefore, rollbackAfter)
	}

	durationBefore := testutil.CollectAndCount(upgradeDurationHistogram)
	podDurationBefore := testutil.CollectAndCount(upgradePodDurationHistogram)
	m.RecordDuration(1.0, "2.4.4", "2.4.5")
	m.RecordPodDuration(0.5, fmt.Sprintf("pod-%s", t.Name()))
	durationAfter := testutil.CollectAndCount(upgradeDurationHistogram)
	podDurationAfter := testutil.CollectAndCount(upgradePodDurationHistogram)
	if durationAfter != durationBefore+1 {
		t.Fatalf("expected upgrade duration histogram series to increase by 1 (before=%d, after=%d)", durationBefore, durationAfter)
	}
	if podDurationAfter != podDurationBefore+1 {
		t.Fatalf("expected upgrade pod duration histogram series to increase by 1 (before=%d, after=%d)", podDurationBefore, podDurationAfter)
	}
}
