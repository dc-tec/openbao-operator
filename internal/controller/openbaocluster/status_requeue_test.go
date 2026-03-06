package openbaocluster

import (
	"testing"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestDetermineStatusRequeue(t *testing.T) {
	t.Parallel()

	r := &OpenBaoClusterReconciler{}

	tests := []struct {
		name     string
		state    *clusterState
		original *openbaov1alpha1.OpenBaoCluster
		cluster  *openbaov1alpha1.OpenBaoCluster
		want     int64
	}{
		{
			name: "nil guards return zero",
			want: 0,
		},
		{
			name: "status stale requeues short",
			state: &clusterState{
				StatusStale: true,
			},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
			want:     int64(constants.RequeueShort),
		},
		{
			name: "partial readiness requeues short",
			state: &clusterState{
				Available:     false,
				ReadyReplicas: 1,
			},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster: &openbaov1alpha1.OpenBaoCluster{Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Replicas: 3,
			}},
			want: int64(constants.RequeueShort),
		},
		{
			name: "availability transition to ready requeues short",
			state: &clusterState{
				Available:     true,
				ReadyReplicas: 3,
			},
			original: &openbaov1alpha1.OpenBaoCluster{Status: openbaov1alpha1.OpenBaoClusterStatus{ReadyReplicas: 2}},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
			want:     int64(constants.RequeueShort),
		},
		{
			name: "no significant change keeps zero result",
			state: &clusterState{
				Available:     true,
				ReadyReplicas: 3,
			},
			original: &openbaov1alpha1.OpenBaoCluster{Status: openbaov1alpha1.OpenBaoClusterStatus{ReadyReplicas: 3}},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
			want:     0,
		},
		{
			name: "no replicas ready does not trigger partial-ready fast requeue",
			state: &clusterState{
				Available:     false,
				ReadyReplicas: 0,
			},
			original: &openbaov1alpha1.OpenBaoCluster{},
			cluster:  &openbaov1alpha1.OpenBaoCluster{},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.determineStatusRequeue(logr.Discard(), tt.state, tt.original, tt.cluster)
			if int64(got.RequeueAfter) != tt.want {
				t.Fatalf("RequeueAfter=%v, want %v", got.RequeueAfter, tt.want)
			}
		})
	}
}

func TestSafetyNetRequeueAfter(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 123456789)
	gotA := safetyNetRequeueAfter(now)
	gotB := safetyNetRequeueAfter(now)
	if gotA != gotB {
		t.Fatalf("expected deterministic output for the same timestamp: %v vs %v", gotA, gotB)
	}

	tests := []time.Time{
		time.Unix(1700000000, 0),
		time.Unix(1700000000, 1),
		time.Unix(1700000000, int64(constants.RequeueSafetyNetJitter/2)),
		time.Unix(1700000000, int64(constants.RequeueSafetyNetJitter-1)),
	}
	for _, ts := range tests {
		got := safetyNetRequeueAfter(ts)
		min := constants.RequeueSafetyNetBase
		maxExclusive := constants.RequeueSafetyNetBase + constants.RequeueSafetyNetJitter
		if got < min || got >= maxExclusive {
			t.Fatalf("safetyNetRequeueAfter(%v)=%v, expected in [%v, %v)", ts, got, min, maxExclusive)
		}
	}
}
