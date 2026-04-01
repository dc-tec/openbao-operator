package core

import (
	"testing"
	"time"
)

func TestReconcileUpgradeMetricsSession_StartsSession(t *testing.T) {
	t.Parallel()

	namespace := "ns-start"
	name := "cluster-start"
	DeleteUpgradeMetricsSession(namespace, name)
	defer DeleteUpgradeMetricsSession(namespace, name)

	now := time.Now()
	transition := ReconcileUpgradeMetricsSession(namespace, name, false, true, false, false, time.Time{}, now)
	if !transition.SessionStarted {
		t.Fatal("expected session start to be reported")
	}
	if transition.RollbackStarted {
		t.Fatal("did not expect rollback start")
	}
	if transition.Completed {
		t.Fatal("did not expect completion")
	}

	state, ok := GetUpgradeMetricsSession(namespace, name)
	if !ok {
		t.Fatal("expected stored metrics session")
	}
	if !state.StartedAt.Equal(now) {
		t.Fatalf("startedAt = %v, want %v", state.StartedAt, now)
	}
}

func TestReconcileUpgradeMetricsSession_RehydratesActiveSessionFromStatusStart(t *testing.T) {
	t.Parallel()

	namespace := "ns-rehydrate"
	name := "cluster-rehydrate"
	DeleteUpgradeMetricsSession(namespace, name)
	defer DeleteUpgradeMetricsSession(namespace, name)

	startedAt := time.Now().Add(-2 * time.Minute)
	now := time.Now()
	transition := ReconcileUpgradeMetricsSession(namespace, name, true, true, false, false, startedAt, now)
	if transition.SessionStarted {
		t.Fatal("did not expect session start for rehydrated active session")
	}

	state, ok := GetUpgradeMetricsSession(namespace, name)
	if !ok {
		t.Fatal("expected stored metrics session")
	}
	if !state.StartedAt.Equal(startedAt) {
		t.Fatalf("startedAt = %v, want %v", state.StartedAt, startedAt)
	}
}

func TestReconcileUpgradeMetricsSession_MarksRollback(t *testing.T) {
	t.Parallel()

	namespace := "ns-rollback"
	name := "cluster-rollback"
	DeleteUpgradeMetricsSession(namespace, name)
	defer DeleteUpgradeMetricsSession(namespace, name)

	startedAt := time.Now().Add(-90 * time.Second)
	SetUpgradeMetricsSession(namespace, name, UpgradeMetricsSessionState{StartedAt: startedAt})

	transition := ReconcileUpgradeMetricsSession(namespace, name, true, true, false, true, startedAt, time.Now())
	if !transition.RollbackStarted {
		t.Fatal("expected rollback start to be reported")
	}

	state, ok := GetUpgradeMetricsSession(namespace, name)
	if !ok {
		t.Fatal("expected stored metrics session")
	}
	if !state.RollbackSeen {
		t.Fatal("expected rollback flag to be persisted")
	}
}

func TestReconcileUpgradeMetricsSession_CompletesAndClearsState(t *testing.T) {
	t.Parallel()

	namespace := "ns-complete"
	name := "cluster-complete"
	DeleteUpgradeMetricsSession(namespace, name)
	defer DeleteUpgradeMetricsSession(namespace, name)

	startedAt := time.Now().Add(-5 * time.Minute)
	SetUpgradeMetricsSession(namespace, name, UpgradeMetricsSessionState{
		StartedAt:    startedAt,
		RollbackSeen: true,
	})

	now := time.Now()
	transition := ReconcileUpgradeMetricsSession(namespace, name, true, false, true, true, startedAt, now)
	if !transition.Completed {
		t.Fatal("expected completion to be reported")
	}
	if !transition.RollbackSeen {
		t.Fatal("expected rollback state to be reported")
	}
	if transition.Duration <= 0 {
		t.Fatalf("duration = %v, want positive duration", transition.Duration)
	}

	if _, ok := GetUpgradeMetricsSession(namespace, name); ok {
		t.Fatal("expected metrics session to be cleared")
	}
}
