package statusops

import (
	"testing"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestDetermineStatusRequeue(t *testing.T) {
	t.Parallel()

	readReplicaCluster := func() *openbaov1alpha1.OpenBaoCluster {
		return &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Replicas:     3,
				ReadReplicas: &openbaov1alpha1.ReadReplicaConfig{Replicas: 2},
			},
		}
	}
	convergedReadState := func() *StatusState {
		return &StatusState{
			Available:                     true,
			ReadyReplicas:                 3,
			ReadReplicaReadyReplicas:      2,
			ReadReplicaRegisteredReplicas: 2,
			ReadReplicaHealthyReplicas:    2,
			ReadServingKnown:              true,
			ReadServingAvailable:          true,
			ReadReplicaMembershipKnown:    true,
			ReadReplicaAutopilotKnown:     true,
		}
	}
	convergedOriginal := func() *openbaov1alpha1.OpenBaoCluster {
		return &openbaov1alpha1.OpenBaoCluster{
			Status: openbaov1alpha1.OpenBaoClusterStatus{
				ReadyReplicas: 3,
				ReadReplicas: &openbaov1alpha1.ReadReplicaStatus{
					ReadyReplicas:      2,
					RegisteredReplicas: 2,
					HealthyReplicas:    2,
				},
			},
		}
	}

	tests := []struct {
		name     string
		state    *StatusState
		original *openbaov1alpha1.OpenBaoCluster
		cluster  *openbaov1alpha1.OpenBaoCluster
		want     time.Duration
	}{
		{name: "nil guards return zero"},
		{
			name:     "stale status requeues",
			state:    &StatusState{StatusStale: true},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
			want:     constants.RequeueShort,
		},
		{
			name:     "partial voter readiness requeues",
			state:    &StatusState{ReadyReplicas: 1},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  &openbaov1alpha1.OpenBaoCluster{Spec: openbaov1alpha1.OpenBaoClusterSpec{Replicas: 3}},
			want:     constants.RequeueShort,
		},
		{
			name:     "voter readiness transition requeues",
			state:    &StatusState{Available: true, ReadyReplicas: 3},
			original: &openbaov1alpha1.OpenBaoCluster{Status: openbaov1alpha1.OpenBaoClusterStatus{ReadyReplicas: 2}},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
			want:     constants.RequeueShort,
		},
		{
			name:     "zero voter readiness does not requeue",
			state:    &StatusState{},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
		},
		{
			name:     "partial read readiness requeues",
			state:    &StatusState{Available: true, ReadyReplicas: 3, ReadReplicaReadyReplicas: 1},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  readReplicaCluster(),
			want:     constants.RequeueShort,
		},
		{
			name:     "unknown read serving requeues",
			state:    &StatusState{Available: true, ReadyReplicas: 3, ReadReplicaReadyReplicas: 2},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  readReplicaCluster(),
			want:     constants.RequeueShort,
		},
		{
			name: "unknown raft membership requeues",
			state: &StatusState{
				Available: true, ReadyReplicas: 3, ReadReplicaReadyReplicas: 2,
				ReadServingKnown: true, ReadServingAvailable: true,
			},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  readReplicaCluster(),
			want:     constants.RequeueShort,
		},
		{
			name: "unknown autopilot health requeues",
			state: &StatusState{
				Available: true, ReadyReplicas: 3, ReadReplicaReadyReplicas: 2,
				ReadReplicaRegisteredReplicas: 2, ReadServingKnown: true, ReadServingAvailable: true,
				ReadReplicaMembershipKnown: true,
			},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  readReplicaCluster(),
			want:     constants.RequeueShort,
		},
		{
			name:  "read replica status transition requeues",
			state: convergedReadState(),
			original: &openbaov1alpha1.OpenBaoCluster{
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					ReadyReplicas: 3,
					ReadReplicas: &openbaov1alpha1.ReadReplicaStatus{
						ReadyReplicas: 1, RegisteredReplicas: 1, HealthyReplicas: 1,
					},
				},
			},
			cluster: readReplicaCluster(),
			want:    constants.RequeueShort,
		},
		{
			name:     "converged status does not requeue",
			state:    convergedReadState(),
			original: convergedOriginal(),
			cluster:  readReplicaCluster(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := determineStatusRequeue(logr.Discard(), tt.state, tt.original, tt.cluster)
			if got.RequeueAfter != tt.want {
				t.Fatalf("RequeueAfter = %v, want %v", got.RequeueAfter, tt.want)
			}
		})
	}
}
