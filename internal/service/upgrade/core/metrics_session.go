package core

import (
	"sync"
	"time"
)

// UpgradeMetricsSessionState tracks in-memory metrics session state that is not
// persisted on the OpenBaoCluster status object.
type UpgradeMetricsSessionState struct {
	StartedAt       time.Time
	StepDownCounted bool
	RollbackSeen    bool
}

var upgradeMetricsSessions sync.Map // key: "namespace/name" -> UpgradeMetricsSessionState

// UpgradeMetricsSessionKey returns the stable in-memory key for a cluster.
func UpgradeMetricsSessionKey(namespace, name string) string {
	return namespace + "/" + name
}

// GetUpgradeMetricsSession returns the stored metrics session state when present.
func GetUpgradeMetricsSession(namespace, name string) (UpgradeMetricsSessionState, bool) {
	value, ok := upgradeMetricsSessions.Load(UpgradeMetricsSessionKey(namespace, name))
	if !ok {
		return UpgradeMetricsSessionState{}, false
	}

	state, ok := value.(UpgradeMetricsSessionState)
	return state, ok
}

// SetUpgradeMetricsSession stores the metrics session state for a cluster.
func SetUpgradeMetricsSession(namespace, name string, state UpgradeMetricsSessionState) {
	upgradeMetricsSessions.Store(UpgradeMetricsSessionKey(namespace, name), state)
}

// DeleteUpgradeMetricsSession clears any stored metrics session state for a cluster.
func DeleteUpgradeMetricsSession(namespace, name string) {
	upgradeMetricsSessions.Delete(UpgradeMetricsSessionKey(namespace, name))
}

// EnsureUpgradeMetricsSession ensures a metrics session exists, initializing it
// with startedAt when absent. The second return value reports whether a new
// session was created.
func EnsureUpgradeMetricsSession(namespace, name string, startedAt time.Time) (UpgradeMetricsSessionState, bool) {
	if state, ok := GetUpgradeMetricsSession(namespace, name); ok {
		return state, false
	}

	state := UpgradeMetricsSessionState{StartedAt: startedAt}
	SetUpgradeMetricsSession(namespace, name, state)
	return state, true
}

// MarkUpgradeMetricsRollbackSeen marks a stored session as having observed a rollback.
// The second return value reports whether a session existed.
func MarkUpgradeMetricsRollbackSeen(namespace, name string) (UpgradeMetricsSessionState, bool) {
	state, ok := GetUpgradeMetricsSession(namespace, name)
	if !ok {
		return UpgradeMetricsSessionState{}, false
	}

	state.RollbackSeen = true
	SetUpgradeMetricsSession(namespace, name, state)
	return state, true
}

// MarkUpgradeMetricsStepDownCounted marks a stored session as having counted a
// step-down. When no session exists, one is created using startedAt. The second
// return value reports whether this call newly marked the step-down.
func MarkUpgradeMetricsStepDownCounted(namespace, name string, startedAt time.Time) (UpgradeMetricsSessionState, bool) {
	state, _ := EnsureUpgradeMetricsSession(namespace, name, startedAt)
	if state.StepDownCounted {
		return state, false
	}

	state.StepDownCounted = true
	SetUpgradeMetricsSession(namespace, name, state)
	return state, true
}
