package snapshot

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
)

func TestReconcileExistingJob(t *testing.T) {
	t.Parallel()

	t.Run("running job uses running handler and is incomplete", func(t *testing.T) {
		t.Parallel()

		calls := make([]string, 0, 2)
		complete, err := ReconcileExistingJob(logr.Discard(), "snapshot-job", JobStateRunning, ExistingJobHandlers{
			OnFound: func(jobName string) { calls = append(calls, "found:"+jobName) },
			OnRunning: func(jobName string) {
				calls = append(calls, "running:"+jobName)
			},
		})
		if err != nil {
			t.Fatalf("ReconcileExistingJob() error = %v, want nil", err)
		}
		if complete {
			t.Fatal("ReconcileExistingJob() complete = true, want false")
		}
		if len(calls) != 2 || calls[0] != "found:snapshot-job" || calls[1] != "running:snapshot-job" {
			t.Fatalf("calls = %v, want found/running", calls)
		}
	})

	t.Run("failed job returns strategy failure result", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("retry exhausted")
		complete, err := ReconcileExistingJob(logr.Discard(), "snapshot-job", JobStateFailed, ExistingJobHandlers{
			OnFailed: func(jobName string) (bool, error) {
				if jobName != "snapshot-job" {
					t.Fatalf("jobName = %q, want snapshot-job", jobName)
				}
				return false, wantErr
			},
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("ReconcileExistingJob() error = %v, want %v", err, wantErr)
		}
		if complete {
			t.Fatal("ReconcileExistingJob() complete = true, want false")
		}
	})

	t.Run("succeeded job returns strategy success result", func(t *testing.T) {
		t.Parallel()

		complete, err := ReconcileExistingJob(logr.Discard(), "snapshot-job", JobStateSucceeded, ExistingJobHandlers{
			OnSucceeded: func(jobName string) (bool, error) {
				if jobName != "snapshot-job" {
					t.Fatalf("jobName = %q, want snapshot-job", jobName)
				}
				return true, nil
			},
		})
		if err != nil {
			t.Fatalf("ReconcileExistingJob() error = %v, want nil", err)
		}
		if !complete {
			t.Fatal("ReconcileExistingJob() complete = false, want true")
		}
	})
}
