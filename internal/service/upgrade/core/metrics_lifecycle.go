package core

import "time"

// UpgradeMetricsPhaseTransition captures the in-memory metrics session changes
// observed during a single strategy reconcile pass.
type UpgradeMetricsPhaseTransition struct {
	SessionStarted  bool
	RollbackStarted bool
	Completed       bool
	Duration        time.Duration
	RollbackSeen    bool
}

// ReconcileUpgradeMetricsSession updates the shared in-memory metrics session
// store for a strategy reconcile pass and reports the observed transitions.
func ReconcileUpgradeMetricsSession(
	namespace string,
	name string,
	initialActive bool,
	active bool,
	initialRollbackSet bool,
	rollbackSet bool,
	startedAt time.Time,
	now time.Time,
) UpgradeMetricsPhaseTransition {
	transition := UpgradeMetricsPhaseTransition{}

	state, ok := GetUpgradeMetricsSession(namespace, name)
	if !ok && initialActive {
		state, _ = EnsureUpgradeMetricsSession(namespace, name, startedAt)
		ok = true
	}

	if !initialActive && active {
		var created bool
		state, created = EnsureUpgradeMetricsSession(namespace, name, now)
		ok = true
		transition.SessionStarted = created
	}

	if rollbackSet && !initialRollbackSet {
		transition.RollbackStarted = true
		if ok {
			state.RollbackSeen = true
			SetUpgradeMetricsSession(namespace, name, state)
		}
	}

	if initialActive && !active && ok {
		transition.Completed = true
		transition.Duration = now.Sub(state.StartedAt)
		transition.RollbackSeen = state.RollbackSeen
		DeleteUpgradeMetricsSession(namespace, name)
	}

	return transition
}
