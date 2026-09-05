package restore

import (
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestDecideRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		state       restoreState
		want        restoreDecisionKind
		wantMessage string
	}{
		{
			name: "marks unknown on observed safety issue before stage routing",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStagePrepared,
				unknownMessage: "identity mismatch",
			},
			want:        restoreDecisionMarkUnknown,
			wantMessage: "identity mismatch",
		},
		{
			name:  "adopts legacy job before stage routing",
			state: restoreState{legacy: true},
			want:  restoreDecisionAdoptLegacyJob,
		},
		{
			name:  "creates prepared execution",
			state: restoreState{executionStage: openbaov1alpha1.RestoreExecutionStagePrepared},
			want:  restoreDecisionCreateJob,
		},
		{
			name: "records committed job before terminal handling",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageCommitted,
				jobState:       restoreJobSucceeded,
			},
			want: restoreDecisionRecordCreatedReceipt,
		},
		{
			name: "polls running created job",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageCreated,
				jobState:       restoreJobRunning,
			},
			want: restoreDecisionPollJob,
		},
		{
			name: "records successful created job",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageCreated,
				jobState:       restoreJobSucceeded,
			},
			want: restoreDecisionRecordSucceededJob,
		},
		{
			name: "records failed created job",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageCreated,
				jobState:       restoreJobFailed,
			},
			want: restoreDecisionRecordFailedJob,
		},
		{
			name: "continues recovery from successful terminal receipt",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageTerminalObserved,
				terminalResult: openbaov1alpha1.RestoreExecutionResultSucceeded,
			},
			want: restoreDecisionContinueRecovery,
		},
		{
			name: "fails restore from failed terminal receipt",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageTerminalObserved,
				terminalResult: openbaov1alpha1.RestoreExecutionResultFailed,
			},
			want: restoreDecisionFailRestore,
		},
		{
			name: "marks unknown for unsupported terminal result",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageTerminalObserved,
				terminalResult: "Other",
			},
			want:        restoreDecisionMarkUnknown,
			wantMessage: "unsupported result",
		},
		{
			name: "completes restore after follow through",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageFollowThroughComplete,
			},
			want: restoreDecisionCompleteRestore,
		},
		{
			name: "idles for unknown execution",
			state: restoreState{
				executionStage: openbaov1alpha1.RestoreExecutionStageUnknown,
			},
			want: restoreDecisionIdle,
		},
		{
			name: "marks unknown for unsupported stage",
			state: restoreState{
				executionStage: "Other",
			},
			want:        restoreDecisionMarkUnknown,
			wantMessage: "unsupported stage",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			decision := decideRestore(test.state)
			if decision.kind != test.want {
				t.Fatalf("decision kind = %d, want %d", decision.kind, test.want)
			}
			if test.wantMessage != "" && !strings.Contains(decision.message, test.wantMessage) {
				t.Fatalf("decision message = %q, want substring %q", decision.message, test.wantMessage)
			}
		})
	}
}
