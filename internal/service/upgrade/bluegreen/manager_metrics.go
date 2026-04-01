package bluegreen

import (
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func (m *Manager) finalizeBlueGreenMetrics(metrics *upgrade.Metrics, strategy string, cluster *openbaov1alpha1.OpenBaoCluster, initialPhase openbaov1alpha1.BlueGreenPhase, initialRollbackSet bool) {
	if metrics == nil || cluster == nil {
		return
	}

	phase := core.CurrentBlueGreenPhase(cluster)
	inProgress := phase != openbaov1alpha1.PhaseIdle
	metrics.SetInProgress(inProgress)
	if inProgress {
		upgrade.SetRunningProgressMetrics(metrics, cluster.Spec.Replicas, 0, 0)
	} else {
		// Leave status unchanged when idle so the last terminal status (success/failed)
		// can be observed after the upgrade completes.
		upgrade.SetInactiveProgressMetrics(metrics)
	}

	startedAt := time.Now()
	if cluster.Status.BlueGreen != nil {
		if cluster.Status.BlueGreen.StartTime != nil {
			startedAt = cluster.Status.BlueGreen.StartTime.Time
		}
	}

	transition := core.ReconcileUpgradeMetricsSession(
		cluster.Namespace,
		cluster.Name,
		initialPhase != openbaov1alpha1.PhaseIdle,
		inProgress,
		initialRollbackSet,
		core.IsBlueGreenRollbackSet(cluster),
		startedAt,
		time.Now(),
	)

	if transition.SessionStarted {
		metrics.IncrementTotal(strategy)
	}

	if transition.RollbackStarted {
		metrics.IncrementRollback(strategy)
		metrics.IncrementFailure(strategy)
	}

	if transition.Completed {
		metrics.RecordDuration(transition.Duration.Seconds(), cluster.Status.CurrentVersion, cluster.Spec.Version)
		if initialPhase == openbaov1alpha1.PhaseCleanup {
			metrics.IncrementSuccess(strategy)
			metrics.SetStatus(upgrade.UpgradeStatusSuccess)
			return
		}

		if !transition.RollbackSeen {
			metrics.IncrementFailure(strategy)
		}
		metrics.SetStatus(upgrade.UpgradeStatusFailed)
	}
}
