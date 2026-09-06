package bluegreen

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestPhaseOutcomeValidate_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome phaseOutcome
		wantErr string
	}{
		{
			name:    "advance requires next phase",
			outcome: phaseOutcome{kind: phaseOutcomeAdvance},
			wantErr: "advance outcome requires nextPhase",
		},
		{
			name:    "advance accepts next phase",
			outcome: advance(openbaov1alpha1.PhaseSyncing),
		},
		{
			name:    "requeue requires positive duration",
			outcome: phaseOutcome{kind: phaseOutcomeRequeueAfter, after: 0},
			wantErr: "requeueAfter outcome requires after > 0",
		},
		{
			name:    "rollback requires reason",
			outcome: phaseOutcome{kind: phaseOutcomeRollback},
			wantErr: "rollback outcome requires reason",
		},
		{
			name:    "abort requires reason",
			outcome: phaseOutcome{kind: phaseOutcomeAbort},
			wantErr: "abort outcome requires reason",
		},
		{
			name:    "hold requires no extra fields",
			outcome: hold(),
		},
		{
			name:    "done requires no extra fields",
			outcome: done(),
		},
		{
			name:    "rejects unknown outcome kind",
			outcome: phaseOutcome{kind: phaseOutcomeKind("unknown")},
			wantErr: "unknown outcome kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.outcome.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("validate() error=nil, want contains %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validate() error=%q, want contains %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestApplyOutcome_StateTransitionInvariants(t *testing.T) {
	t.Parallel()

	baseCluster := func() *openbaov1alpha1.OpenBaoCluster {
		start := metav1.NewTime(time.Now().Add(-2 * time.Minute))
		return &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster-a",
				Namespace: "tenant-a",
			},
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				BlueGreen: &openbaov1alpha1.BlueGreenStatus{
					Phase:           openbaov1alpha1.PhaseJoiningMesh,
					StartTime:       &start,
					JobFailureCount: 3,
					LastJobFailure:  "join-mesh-job",
				},
			},
		}
	}

	tests := []struct {
		name      string
		outcome   phaseOutcome
		wantErr   string
		assertion func(t *testing.T, resultAfter time.Duration, cluster *openbaov1alpha1.OpenBaoCluster, before *openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name:    "advance resets failure counters and requeues",
			outcome: advance(openbaov1alpha1.PhaseSyncing),
			assertion: func(t *testing.T, resultAfter time.Duration, cluster *openbaov1alpha1.OpenBaoCluster, _ *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseSyncing {
					t.Fatalf("phase=%s, want %s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseSyncing)
				}
				if cluster.Status.BlueGreen.StartTime == nil {
					t.Fatalf("StartTime=nil, want non-nil on non-idle phase")
				}
				if cluster.Status.BlueGreen.JobFailureCount != 0 {
					t.Fatalf("JobFailureCount=%d, want 0", cluster.Status.BlueGreen.JobFailureCount)
				}
				if cluster.Status.BlueGreen.LastJobFailure != "" {
					t.Fatalf("LastJobFailure=%q, want empty", cluster.Status.BlueGreen.LastJobFailure)
				}
				if resultAfter != constants.RequeueShort {
					t.Fatalf("RequeueAfter=%s, want %s", resultAfter, constants.RequeueShort)
				}
			},
		},
		{
			name:    "advance to idle clears start time and does not requeue",
			outcome: advance(openbaov1alpha1.PhaseIdle),
			assertion: func(t *testing.T, resultAfter time.Duration, cluster *openbaov1alpha1.OpenBaoCluster, _ *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster.Status.BlueGreen.Phase != openbaov1alpha1.PhaseIdle {
					t.Fatalf("phase=%s, want %s", cluster.Status.BlueGreen.Phase, openbaov1alpha1.PhaseIdle)
				}
				if cluster.Status.BlueGreen.StartTime != nil {
					t.Fatalf("StartTime=%v, want nil at idle", cluster.Status.BlueGreen.StartTime)
				}
				if cluster.Status.BlueGreen.JobFailureCount != 0 {
					t.Fatalf("JobFailureCount=%d, want 0", cluster.Status.BlueGreen.JobFailureCount)
				}
				if cluster.Status.BlueGreen.LastJobFailure != "" {
					t.Fatalf("LastJobFailure=%q, want empty", cluster.Status.BlueGreen.LastJobFailure)
				}
				if resultAfter != 0 {
					t.Fatalf("RequeueAfter=%s, want 0", resultAfter)
				}
			},
		},
		{
			name:    "hold keeps state unchanged",
			outcome: hold(),
			assertion: func(t *testing.T, resultAfter time.Duration, cluster *openbaov1alpha1.OpenBaoCluster, before *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster.Status.BlueGreen.Phase != before.Status.BlueGreen.Phase {
					t.Fatalf("phase=%s, want unchanged %s", cluster.Status.BlueGreen.Phase, before.Status.BlueGreen.Phase)
				}
				if cluster.Status.BlueGreen.JobFailureCount != before.Status.BlueGreen.JobFailureCount {
					t.Fatalf("JobFailureCount=%d, want unchanged %d", cluster.Status.BlueGreen.JobFailureCount, before.Status.BlueGreen.JobFailureCount)
				}
				if cluster.Status.BlueGreen.LastJobFailure != before.Status.BlueGreen.LastJobFailure {
					t.Fatalf("LastJobFailure=%q, want unchanged %q", cluster.Status.BlueGreen.LastJobFailure, before.Status.BlueGreen.LastJobFailure)
				}
				if resultAfter != 0 {
					t.Fatalf("RequeueAfter=%s, want 0", resultAfter)
				}
			},
		},
		{
			name:    "requeueAfter keeps state unchanged",
			outcome: requeueAfterOutcome(7 * time.Second),
			assertion: func(t *testing.T, resultAfter time.Duration, cluster *openbaov1alpha1.OpenBaoCluster, before *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster.Status.BlueGreen.Phase != before.Status.BlueGreen.Phase {
					t.Fatalf("phase=%s, want unchanged %s", cluster.Status.BlueGreen.Phase, before.Status.BlueGreen.Phase)
				}
				if cluster.Status.BlueGreen.JobFailureCount != before.Status.BlueGreen.JobFailureCount {
					t.Fatalf("JobFailureCount=%d, want unchanged %d", cluster.Status.BlueGreen.JobFailureCount, before.Status.BlueGreen.JobFailureCount)
				}
				if resultAfter != 7*time.Second {
					t.Fatalf("RequeueAfter=%s, want %s", resultAfter, 7*time.Second)
				}
			},
		},
		{
			name:    "invalid outcome is rejected without mutation",
			outcome: phaseOutcome{kind: phaseOutcomeAdvance},
			wantErr: "advance outcome requires nextPhase",
			assertion: func(t *testing.T, _ time.Duration, cluster *openbaov1alpha1.OpenBaoCluster, before *openbaov1alpha1.OpenBaoCluster) {
				t.Helper()
				if cluster.Status.BlueGreen.Phase != before.Status.BlueGreen.Phase {
					t.Fatalf("phase mutated to %s, want unchanged %s", cluster.Status.BlueGreen.Phase, before.Status.BlueGreen.Phase)
				}
				if cluster.Status.BlueGreen.JobFailureCount != before.Status.BlueGreen.JobFailureCount {
					t.Fatalf("JobFailureCount mutated to %d, want unchanged %d", cluster.Status.BlueGreen.JobFailureCount, before.Status.BlueGreen.JobFailureCount)
				}
				if cluster.Status.BlueGreen.LastJobFailure != before.Status.BlueGreen.LastJobFailure {
					t.Fatalf("LastJobFailure mutated to %q, want unchanged %q", cluster.Status.BlueGreen.LastJobFailure, before.Status.BlueGreen.LastJobFailure)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cluster := baseCluster()
			before := cluster.DeepCopy()
			mgr := &Manager{}

			result, err := mgr.applyOutcome(context.Background(), logr.Discard(), cluster, tt.outcome)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("applyOutcome() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("applyOutcome() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("applyOutcome() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
			}

			if tt.assertion != nil {
				tt.assertion(t, result.RequeueAfter, cluster, before)
			}
		})
	}
}

func TestExecuteStateMachine_UnknownPhaseRejected(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "tenant-a",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				Phase: openbaov1alpha1.BlueGreenPhase("invalid-phase"),
			},
		},
	}
	before := cluster.DeepCopy()

	mgr := &Manager{}
	_, err := mgr.executeStateMachine(context.Background(), logr.Discard(), cluster, "", &upgrade.RequestAcknowledgements{})
	if err == nil {
		t.Fatalf("executeStateMachine() error=nil, want unknown phase error")
	}
	if !strings.Contains(err.Error(), "unknown blue/green phase") {
		t.Fatalf("executeStateMachine() error=%q, want contains %q", err.Error(), "unknown blue/green phase")
	}
	if cluster.Status.BlueGreen.Phase != before.Status.BlueGreen.Phase {
		t.Fatalf("phase mutated to %s, want unchanged %s", cluster.Status.BlueGreen.Phase, before.Status.BlueGreen.Phase)
	}
}
