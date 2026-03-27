package bluegreen

import (
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func (m *Manager) finalizeBlueGreenMetrics(metrics *upgrade.Metrics, strategy string, cluster *openbaov1alpha1.OpenBaoCluster, initialPhase openbaov1alpha1.BlueGreenPhase, initialRollbackSet bool) {
	if metrics == nil || cluster == nil {
		return
	}

	phase := openbaov1alpha1.PhaseIdle
	if cluster.Status.BlueGreen != nil {
		phase = cluster.Status.BlueGreen.Phase
	}

	inProgress := phase != openbaov1alpha1.PhaseIdle
	metrics.SetInProgress(inProgress)
	if inProgress {
		metrics.SetStatus(upgrade.UpgradeStatusRunning)
		metrics.SetTotalPods(int(cluster.Spec.Replicas))
	} else {
		// Leave status unchanged when idle so the last terminal status (success/failed)
		// can be observed after the upgrade completes.
		metrics.SetTotalPods(0)
	}
	metrics.SetPodsCompleted(0)
	metrics.SetPartition(0)

	state, ok := getUpgradeMetricsState(cluster.Namespace, cluster.Name)
	if !ok && initialPhase != openbaov1alpha1.PhaseIdle {
		startedAt := time.Now()
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.StartTime != nil {
			startedAt = cluster.Status.BlueGreen.StartTime.Time
		}
		state = upgradeMetricsState{startedAt: startedAt}
		ok = true
		setUpgradeMetricsState(cluster.Namespace, cluster.Name, state)
	}

	// If a new upgrade started this reconcile, initialize state and increment totals once.
	if initialPhase == openbaov1alpha1.PhaseIdle && phase != openbaov1alpha1.PhaseIdle {
		if _, exists := getUpgradeMetricsState(cluster.Namespace, cluster.Name); !exists {
			setUpgradeMetricsState(cluster.Namespace, cluster.Name, upgradeMetricsState{startedAt: time.Now()})
			metrics.IncrementTotal(strategy)
			state, ok = getUpgradeMetricsState(cluster.Namespace, cluster.Name)
		}
	}

	// Rollback initiation: count once when RollbackStartTime is first set.
	if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.RollbackStartTime != nil && !initialRollbackSet {
		if ok {
			state.lastRollbackSeen = true
			setUpgradeMetricsState(cluster.Namespace, cluster.Name, state)
		}
		metrics.IncrementRollback(strategy)
		metrics.IncrementFailure(strategy)
	}

	// Completion: a transition from any non-idle phase to idle.
	if initialPhase != openbaov1alpha1.PhaseIdle && phase == openbaov1alpha1.PhaseIdle && ok {
		durationSeconds := time.Since(state.startedAt).Seconds()
		metrics.RecordDuration(durationSeconds, cluster.Status.CurrentVersion, cluster.Spec.Version)
		deleteUpgradeMetricsState(cluster.Namespace, cluster.Name)

		if initialPhase == openbaov1alpha1.PhaseCleanup {
			metrics.IncrementSuccess(strategy)
			metrics.SetStatus(upgrade.UpgradeStatusSuccess)
			return
		}

		if !state.lastRollbackSeen {
			metrics.IncrementFailure(strategy)
		}
		metrics.SetStatus(upgrade.UpgradeStatusFailed)
	}
}
