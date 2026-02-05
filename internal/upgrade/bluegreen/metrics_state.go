package bluegreen

import (
	"sync"
	"time"
)

type upgradeMetricsState struct {
	startedAt        time.Time
	stepDownCounted  bool
	lastRollbackSeen bool
}

var blueGreenUpgradeMetricsState sync.Map // key: "namespace/name" -> upgradeMetricsState

func metricsStateKey(namespace, name string) string {
	return namespace + "/" + name
}

func getUpgradeMetricsState(namespace, name string) (upgradeMetricsState, bool) {
	value, ok := blueGreenUpgradeMetricsState.Load(metricsStateKey(namespace, name))
	if !ok {
		return upgradeMetricsState{}, false
	}
	state, ok := value.(upgradeMetricsState)
	return state, ok
}

func setUpgradeMetricsState(namespace, name string, state upgradeMetricsState) {
	blueGreenUpgradeMetricsState.Store(metricsStateKey(namespace, name), state)
}

func deleteUpgradeMetricsState(namespace, name string) {
	blueGreenUpgradeMetricsState.Delete(metricsStateKey(namespace, name))
}
