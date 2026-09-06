package core_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

func TestCurrentBlueGreenPhaseDefaultsToIdle(t *testing.T) {
	t.Parallel()

	require.Equal(t, openbaov1alpha1.PhaseIdle, core.CurrentBlueGreenPhase(nil))
	require.Equal(t, openbaov1alpha1.PhaseIdle, core.CurrentBlueGreenPhase(&openbaov1alpha1.OpenBaoCluster{}))
}

func TestBlueGreenUpgradeState(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.0",
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase: openbaov1alpha1.PhaseSyncing,
			},
		},
	}

	active, needed := core.BlueGreenUpgradeState(cluster)
	require.True(t, active)
	require.True(t, needed)
	require.False(t, core.IsBlueGreenRollbackSet(cluster))
}

func TestInitializeBlueGreenManualPromotion(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Upgrade: &openbaov1alpha1.UpgradeConfig{
				BlueGreen: &openbaov1alpha1.BlueGreenConfig{
					AutoPromote: false,
				},
			},
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{},
		},
	}

	require.True(t, core.BlueGreenStartEventPending(cluster))
	core.InitializeBlueGreenManualPromotion(cluster)
	require.True(t, cluster.Status.BlueGreen.ManualPromotionRequired)
}

func TestFinalizeBlueGreenTerminalState(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Image: "openbao:2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase:                   openbaov1alpha1.PhaseCleanup,
				BlueRevision:            "blue",
				GreenRevision:           "green",
				StartTime:               &now,
				ManualPromotionRequired: true,
				JobFailureCount:         2,
				LastJobFailure:          "boom",
				RollbackStartTime:       &now,
			},
		},
	}

	core.FinalizeBlueGreenTerminalState(cluster, true)

	require.Equal(t, openbaov1alpha1.PhaseIdle, cluster.Status.BlueGreen.Phase)
	require.Equal(t, "green", cluster.Status.BlueGreen.BlueRevision)
	require.Equal(t, "openbao:2.5.0", cluster.Status.BlueGreen.BlueImage)
	require.Empty(t, cluster.Status.BlueGreen.GreenRevision)
	require.False(t, cluster.Status.BlueGreen.ManualPromotionRequired)
	require.Nil(t, cluster.Status.BlueGreen.StartTime)
	require.Zero(t, cluster.Status.BlueGreen.JobFailureCount)
	require.Empty(t, cluster.Status.BlueGreen.LastJobFailure)
	require.NotNil(t, cluster.Status.BlueGreen.RollbackStartTime)
}

func TestResetBlueGreenTransientStatePreservesOperationUntilHookCleanup(t *testing.T) {
	t.Parallel()

	status := &openbaov1alpha1.BlueGreenStatus{
		Phase:       openbaov1alpha1.PhaseSyncing,
		OperationID: "operation-1",
		ValidationHook: &openbaov1alpha1.BlueGreenValidationHookStatus{
			OperationID: "operation-1",
			Stage:       openbaov1alpha1.BlueGreenValidationHookStageTerminalObserved,
			JobName:     "validation-hook",
		},
	}
	core.ResetBlueGreenTransientState(status)
	require.Equal(t, "operation-1", status.OperationID)
	require.NotNil(t, status.ValidationHook)

	status.ValidationHook = nil
	core.ResetBlueGreenTransientState(status)
	require.Empty(t, status.OperationID)
}

func TestBlueGreenTransitionState(t *testing.T) {
	t.Parallel()
	const (
		clearTimer    = "clear"
		restartTimer  = "restart"
		preserveTimer = "preserve"
	)
	tests := []struct {
		name          string
		transition    func(*openbaov1alpha1.BlueGreenStatus)
		wantPhase     openbaov1alpha1.BlueGreenPhase
		phaseTimer    string
		rollbackTimer string
		clearFailures bool
		terminal      bool
	}{
		{
			name:       "advance to syncing",
			transition: func(s *openbaov1alpha1.BlueGreenStatus) { core.AdvanceBlueGreenPhase(s, openbaov1alpha1.PhaseSyncing) },
			wantPhase:  openbaov1alpha1.PhaseSyncing, phaseTimer: restartTimer, rollbackTimer: preserveTimer, clearFailures: true,
		},
		{
			name: "advance to read replica restore",
			transition: func(s *openbaov1alpha1.BlueGreenStatus) {
				core.AdvanceBlueGreenPhase(s, openbaov1alpha1.PhaseRestoringReadReplicas)
			},
			wantPhase: openbaov1alpha1.PhaseRestoringReadReplicas, phaseTimer: restartTimer, rollbackTimer: preserveTimer, clearFailures: true,
		},
		{
			name:       "start rollback retains phase timer and failures",
			transition: func(s *openbaov1alpha1.BlueGreenStatus) { core.BeginBlueGreenRollback(s, "new rollback") },
			wantPhase:  openbaov1alpha1.PhaseRollingBack, phaseTimer: preserveTimer, rollbackTimer: restartTimer,
		},
		{
			name: "advance to rollback cleanup",
			transition: func(s *openbaov1alpha1.BlueGreenStatus) {
				core.AdvanceBlueGreenPhase(s, openbaov1alpha1.PhaseRollbackCleanup)
			},
			wantPhase: openbaov1alpha1.PhaseRollbackCleanup, phaseTimer: restartTimer, rollbackTimer: preserveTimer, clearFailures: true,
		},
		{
			name:       "advance to idle only resets phase state",
			transition: func(s *openbaov1alpha1.BlueGreenStatus) { core.AdvanceBlueGreenPhase(s, openbaov1alpha1.PhaseIdle) },
			wantPhase:  openbaov1alpha1.PhaseIdle, phaseTimer: clearTimer, rollbackTimer: preserveTimer, clearFailures: true,
		},
		{
			name:       "terminal reset retains rollback history",
			transition: core.ResetBlueGreenTransientState,
			wantPhase:  openbaov1alpha1.PhaseIdle, phaseTimer: clearTimer, rollbackTimer: preserveTimer, clearFailures: true, terminal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			old := metav1.NewTime(time.Now().Add(-time.Hour))
			status := &openbaov1alpha1.BlueGreenStatus{
				Phase:     openbaov1alpha1.PhasePromoting,
				StartTime: &old, JobFailureCount: 3, LastJobFailure: "failed-job",
				RollbackStartTime: &old, RollbackReason: "old rollback", RollbackAttempt: 2,
				GreenRevision: "green", BlueRevision: "blue", BlueImage: "openbao:2.4.0",
				OperationID: "operation-1", ManualPromotionRequired: true,
			}
			before := status.DeepCopy()
			started := time.Now()
			tt.transition(status)
			finished := time.Now()
			require.Equal(t, tt.wantPhase, status.Phase)
			assertTimer := func(timer *metav1.Time, policy string) {
				t.Helper()
				switch policy {
				case clearTimer:
					require.Nil(t, timer)
				case preserveTimer:
					require.Equal(t, &old, timer)
				case restartTimer:
					require.NotNil(t, timer)
					require.False(t, timer.Time.Before(started))
					require.False(t, timer.After(finished))
				}
			}
			assertTimer(status.StartTime, tt.phaseTimer)
			assertTimer(status.RollbackStartTime, tt.rollbackTimer)
			if tt.clearFailures {
				require.Zero(t, status.JobFailureCount)
				require.Empty(t, status.LastJobFailure)
			} else {
				require.Equal(t, before.JobFailureCount, status.JobFailureCount)
				require.Equal(t, before.LastJobFailure, status.LastJobFailure)
			}
			if tt.rollbackTimer == restartTimer {
				require.Equal(t, "new rollback", status.RollbackReason)
			} else {
				require.Equal(t, before.RollbackReason, status.RollbackReason)
			}
			require.Equal(t, before.RollbackAttempt, status.RollbackAttempt)
			require.Equal(t, before.BlueRevision, status.BlueRevision)
			require.Equal(t, before.BlueImage, status.BlueImage)
			if tt.terminal {
				require.Empty(t, status.GreenRevision)
				require.Empty(t, status.OperationID)
				require.False(t, status.ManualPromotionRequired)
			} else {
				require.Equal(t, before.GreenRevision, status.GreenRevision)
				require.Equal(t, before.OperationID, status.OperationID)
				require.Equal(t, before.ManualPromotionRequired, status.ManualPromotionRequired)
			}
		})
	}
}
