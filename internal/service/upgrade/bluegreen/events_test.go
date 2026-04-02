package bluegreen

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func expectEventContains(t *testing.T, recorder *events.FakeRecorder, parts ...string) {
	t.Helper()

	select {
	case event := <-recorder.Events:
		for _, part := range parts {
			if !strings.Contains(event, part) {
				t.Fatalf("event %q does not contain %q", event, part)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("expected event, got none")
	}
}

func TestHandlePhaseSyncing_EmitsManualHoldAndPromotionEvents(t *testing.T) {
	t.Parallel()

	t.Run("manual hold", func(t *testing.T) {
		t.Parallel()

		scheme := newBlueGreenTestScheme(t)
		cluster := newPhaseMachineCluster()
		cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseSyncing
		cluster.Status.BlueGreen.ManualPromotionRequired = true
		job := succeededExecutorJob(cluster, ActionWaitGreenSynced)
		recorder := events.NewFakeRecorder(10)
		manager := &Manager{
			client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build(),
			scheme:   scheme,
			recorder: recorder,
		}

		outcome, err := manager.handlePhaseSyncing(context.Background(), logr.Discard(), cluster)
		if err != nil {
			t.Fatalf("handlePhaseSyncing() error = %v", err)
		}
		if outcome.kind != phaseOutcomeHold {
			t.Fatalf("handlePhaseSyncing() outcome = %+v, want hold", outcome)
		}

		expectEventContains(t, recorder, "Normal", ReasonBlueGreenHoldEntered)
	})

	t.Run("promotion approved", func(t *testing.T) {
		t.Parallel()

		scheme := newBlueGreenTestScheme(t)
		cluster := newPhaseMachineCluster()
		cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseSyncing
		cluster.Status.BlueGreen.ManualPromotionRequired = true
		cluster.Spec.Upgrade.Requests = &openbaov1alpha1.UpgradeRequestConfig{
			Promote: "2026-03-10T12:00:00Z",
		}
		job := succeededExecutorJob(cluster, ActionWaitGreenSynced)
		recorder := events.NewFakeRecorder(10)
		manager := &Manager{
			client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build(),
			scheme:   scheme,
			recorder: recorder,
		}

		outcome, err := manager.handlePhaseSyncing(context.Background(), logr.Discard(), cluster)
		if err != nil {
			t.Fatalf("handlePhaseSyncing() error = %v", err)
		}
		if outcome.kind != phaseOutcomeAdvance || outcome.nextPhase != openbaov1alpha1.PhasePromoting {
			t.Fatalf("handlePhaseSyncing() outcome = %+v, want advance to promoting", outcome)
		}
		if cluster.Status.UpgradeRequests == nil || cluster.Status.UpgradeRequests.LastHandledPromote != "2026-03-10T12:00:00Z" {
			t.Fatalf("LastHandledPromote = %+v, want request to be recorded", cluster.Status.UpgradeRequests)
		}

		expectEventContains(t, recorder, "Normal", ReasonBlueGreenPromotionApproved)
	})
}

func TestTriggerRollback_EmitsRollbackStartedEvent(t *testing.T) {
	t.Parallel()

	cluster := newBlueGreenCluster()
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseCleanup
	recorder := events.NewFakeRecorder(10)
	manager := &Manager{recorder: recorder}

	result, err := manager.triggerRollback(logr.Discard(), cluster, "cleanup failed")
	if err != nil {
		t.Fatalf("triggerRollback() error = %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("triggerRollback() result = %+v, want positive requeue", result)
	}

	expectEventContains(t, recorder, "Warning", ReasonRollbackStarted)
}

func TestHandleManualRollbackRequest_EmitsRollbackStartedEvent(t *testing.T) {
	t.Parallel()

	cluster := newBlueGreenCluster()
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseCleanup
	cluster.Status.BlueGreen.GreenRevision = DeploymentNameSuffix
	cluster.Spec.Upgrade.Requests = &openbaov1alpha1.UpgradeRequestConfig{
		Rollback: "2026-03-10T12:05:00Z",
	}
	recorder := events.NewFakeRecorder(10)
	manager := &Manager{recorder: recorder}

	handled, result, err := manager.handleManualRollbackRequest(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("handleManualRollbackRequest() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("result = %+v, want positive requeue", result)
	}
	if cluster.Status.UpgradeRequests == nil || cluster.Status.UpgradeRequests.LastHandledRollback != "2026-03-10T12:05:00Z" {
		t.Fatalf("LastHandledRollback = %+v, want request to be recorded", cluster.Status.UpgradeRequests)
	}

	expectEventContains(t, recorder, "Warning", ReasonRollbackStarted)
}

func TestHandleManualRollbackRequest_IgnoresStaleRequestWhenIdle(t *testing.T) {
	t.Parallel()

	cluster := newBlueGreenCluster()
	cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseIdle
	cluster.Spec.Upgrade.Requests = &openbaov1alpha1.UpgradeRequestConfig{
		Rollback: "2026-03-10T12:15:00Z",
	}
	manager := &Manager{}

	handled, result, err := manager.handleManualRollbackRequest(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("handleManualRollbackRequest() error = %v", err)
	}
	if handled {
		t.Fatalf("handled = true, want false with result %+v", result)
	}
	if cluster.Status.UpgradeRequests == nil || cluster.Status.UpgradeRequests.LastHandledRollback != "2026-03-10T12:15:00Z" {
		t.Fatalf("LastHandledRollback = %+v, want request to be recorded", cluster.Status.UpgradeRequests)
	}
}

func TestBreakGlassEvents(t *testing.T) {
	t.Parallel()

	t.Run("entered", func(t *testing.T) {
		t.Parallel()

		cluster := newBlueGreenCluster()
		recorder := events.NewFakeRecorder(10)
		manager := &Manager{recorder: recorder}

		manager.enterBreakGlassRollbackConsensusRepairFailed(logr.Discard(), cluster, "rollback-job")

		if cluster.Status.BreakGlass == nil || !cluster.Status.BreakGlass.Active {
			t.Fatal("break glass not activated")
		}

		expectEventContains(t, recorder, "Warning", ReasonBreakGlassEntered)
	})

	t.Run("entered for rollback cleanup peer removal", func(t *testing.T) {
		t.Parallel()

		cluster := newBlueGreenCluster()
		recorder := events.NewFakeRecorder(10)
		manager := &Manager{recorder: recorder}

		manager.enterBreakGlassRollbackCleanupPeerRemovalFailed(logr.Discard(), cluster, "rollback-cleanup-job")

		if cluster.Status.BreakGlass == nil || !cluster.Status.BreakGlass.Active {
			t.Fatal("break glass not activated")
		}

		expectEventContains(t, recorder, "Warning", ReasonBreakGlassEntered)
	})

	t.Run("acknowledged", func(t *testing.T) {
		t.Parallel()

		cluster := newBlueGreenCluster()
		cluster.Status.BreakGlass = &openbaov1alpha1.BreakGlassStatus{
			Active: true,
			Nonce:  "nonce-1",
			Reason: openbaov1alpha1.BreakGlassReasonRollbackConsensusRepairFailed,
		}
		cluster.Spec.BreakGlassAck = "nonce-1"
		cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseRollingBack

		recorder := events.NewFakeRecorder(10)
		manager := &Manager{recorder: recorder}

		handled, result := manager.handleBreakGlassAck(logr.Discard(), cluster)
		if !handled {
			t.Fatal("handled = false, want true")
		}
		if result.RequeueAfter <= 0 {
			t.Fatalf("result = %+v, want positive requeue", result)
		}

		expectEventContains(t, recorder, "Normal", ReasonBreakGlassAcknowledged)
	})
}
