package core

import (
	"testing"
	"time"
)

func TestUpgradeMetricsSessionStore_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		cluster   string
		state     UpgradeMetricsSessionState
	}{
		{
			name:      "stores and retrieves first state",
			namespace: "ns-a",
			cluster:   "cluster-a",
			state: UpgradeMetricsSessionState{
				StartedAt:       time.Unix(100, 0),
				StepDownCounted: true,
				RollbackSeen:    false,
			},
		},
		{
			name:      "stores and retrieves second state",
			namespace: "ns-b",
			cluster:   "cluster-b",
			state: UpgradeMetricsSessionState{
				StartedAt:       time.Unix(200, 0),
				StepDownCounted: false,
				RollbackSeen:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			DeleteUpgradeMetricsSession(tt.namespace, tt.cluster)

			if _, ok := GetUpgradeMetricsSession(tt.namespace, tt.cluster); ok {
				t.Fatalf("expected state to be absent before set")
			}

			SetUpgradeMetricsSession(tt.namespace, tt.cluster, tt.state)
			got, ok := GetUpgradeMetricsSession(tt.namespace, tt.cluster)
			if !ok {
				t.Fatalf("expected state to be present after set")
			}
			if !got.StartedAt.Equal(tt.state.StartedAt) ||
				got.StepDownCounted != tt.state.StepDownCounted ||
				got.RollbackSeen != tt.state.RollbackSeen {
				t.Fatalf("got state %+v, want %+v", got, tt.state)
			}

			DeleteUpgradeMetricsSession(tt.namespace, tt.cluster)
			if _, ok := GetUpgradeMetricsSession(tt.namespace, tt.cluster); ok {
				t.Fatalf("expected state to be absent after delete")
			}
		})
	}
}

func TestEnsureAndMarkUpgradeMetricsSession(t *testing.T) {
	t.Parallel()

	namespace := "metrics"
	cluster := "cluster"
	startedAt := time.Unix(300, 0)
	DeleteUpgradeMetricsSession(namespace, cluster)
	defer DeleteUpgradeMetricsSession(namespace, cluster)

	state, created := EnsureUpgradeMetricsSession(namespace, cluster, startedAt)
	if !created {
		t.Fatal("expected first ensure to create session")
	}
	if !state.StartedAt.Equal(startedAt) {
		t.Fatalf("StartedAt = %v, want %v", state.StartedAt, startedAt)
	}

	state, created = EnsureUpgradeMetricsSession(namespace, cluster, time.Unix(400, 0))
	if created {
		t.Fatal("expected second ensure to reuse session")
	}
	if !state.StartedAt.Equal(startedAt) {
		t.Fatalf("StartedAt = %v, want original %v", state.StartedAt, startedAt)
	}

	state, marked := MarkUpgradeMetricsStepDownCounted(namespace, cluster, time.Unix(500, 0))
	if !marked {
		t.Fatal("expected first step-down mark to succeed")
	}
	if !state.StepDownCounted {
		t.Fatal("expected step-down to be marked")
	}

	_, marked = MarkUpgradeMetricsStepDownCounted(namespace, cluster, time.Unix(600, 0))
	if marked {
		t.Fatal("expected second step-down mark to be ignored")
	}

	state, ok := MarkUpgradeMetricsRollbackSeen(namespace, cluster)
	if !ok {
		t.Fatal("expected rollback marker to find session")
	}
	if !state.RollbackSeen {
		t.Fatal("expected rollback to be marked")
	}
}
