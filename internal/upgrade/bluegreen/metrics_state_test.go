package bluegreen

import (
	"testing"
	"time"
)

func TestUpgradeMetricsStateStore_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		cluster   string
		state     upgradeMetricsState
	}{
		{
			name:      "stores and retrieves first state",
			namespace: "ns-a",
			cluster:   "cluster-a",
			state: upgradeMetricsState{
				startedAt:        time.Unix(100, 0),
				stepDownCounted:  true,
				lastRollbackSeen: false,
			},
		},
		{
			name:      "stores and retrieves second state",
			namespace: "ns-b",
			cluster:   "cluster-b",
			state: upgradeMetricsState{
				startedAt:        time.Unix(200, 0),
				stepDownCounted:  false,
				lastRollbackSeen: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			deleteUpgradeMetricsState(tt.namespace, tt.cluster)

			if _, ok := getUpgradeMetricsState(tt.namespace, tt.cluster); ok {
				t.Fatalf("expected state to be absent before set")
			}

			setUpgradeMetricsState(tt.namespace, tt.cluster, tt.state)
			got, ok := getUpgradeMetricsState(tt.namespace, tt.cluster)
			if !ok {
				t.Fatalf("expected state to be present after set")
			}
			if !got.startedAt.Equal(tt.state.startedAt) ||
				got.stepDownCounted != tt.state.stepDownCounted ||
				got.lastRollbackSeen != tt.state.lastRollbackSeen {
				t.Fatalf("got state %+v, want %+v", got, tt.state)
			}

			deleteUpgradeMetricsState(tt.namespace, tt.cluster)
			if _, ok := getUpgradeMetricsState(tt.namespace, tt.cluster); ok {
				t.Fatalf("expected state to be absent after delete")
			}
		})
	}
}
