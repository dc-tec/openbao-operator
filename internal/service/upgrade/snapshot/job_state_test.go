package snapshot

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func TestJobStateFromBatchJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		job  *batchv1.Job
		want JobState
	}{
		{name: "nil job", job: nil, want: JobStateNone},
		{
			name: "succeeded job",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}},
					Succeeded:  1,
				},
			},
			want: JobStateSucceeded,
		},
		{
			name: "failed job",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}},
					Failed:     1,
				},
			},
			want: JobStateFailed,
		},
		{
			name: "running job",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{Active: 1},
			},
			want: JobStateRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := JobStateFromBatchJob(tt.job); got != tt.want {
				t.Fatalf("JobStateFromBatchJob() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJobStateFromResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result *upgrade.JobResult
		want   JobState
	}{
		{name: "nil result", result: nil, want: JobStateNone},
		{name: "missing result", result: &upgrade.JobResult{}, want: JobStateNone},
		{name: "running result", result: &upgrade.JobResult{Exists: true, Running: true}, want: JobStateRunning},
		{name: "failed result", result: &upgrade.JobResult{Exists: true, Failed: true}, want: JobStateFailed},
		{name: "succeeded result", result: &upgrade.JobResult{Exists: true, Succeeded: true}, want: JobStateSucceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := JobStateFromResult(tt.result); got != tt.want {
				t.Fatalf("JobStateFromResult() = %q, want %q", got, tt.want)
			}
		})
	}
}
