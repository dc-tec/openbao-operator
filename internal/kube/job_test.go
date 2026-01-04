package kube

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestJobSucceeded(t *testing.T) {
	tests := []struct {
		name string
		job  *batchv1.Job
		want bool
	}{
		{name: "nil", job: nil, want: false},
		{name: "condition complete", job: &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}}, want: true},
		{name: "succeeded count", job: &batchv1.Job{Status: batchv1.JobStatus{Succeeded: 1}}, want: true},
		{name: "not succeeded", job: &batchv1.Job{Status: batchv1.JobStatus{}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JobSucceeded(tt.job); got != tt.want {
				t.Fatalf("JobSucceeded()=%t, want %t", got, tt.want)
			}
		})
	}
}

func TestJobFailed(t *testing.T) {
	tests := []struct {
		name string
		job  *batchv1.Job
		want bool
	}{
		{name: "nil", job: nil, want: false},
		{name: "condition failed", job: &batchv1.Job{Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}}}, want: true},
		{
			name: "fallback failed terminal",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed:    1,
					Active:    0,
					Succeeded: 0,
				},
			},
			want: true,
		},
		{
			name: "failed but still active",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed: 1,
					Active: 1,
				},
			},
			want: false,
		},
		{
			name: "succeeded even with failed count",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed:    1,
					Active:    0,
					Succeeded: 1,
					Conditions: []batchv1.JobCondition{
						{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: metav1.Now()},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JobFailed(tt.job); got != tt.want {
				t.Fatalf("JobFailed()=%t, want %t", got, tt.want)
			}
		})
	}
}
