package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
)

type fakeRaftPeerDemoter struct {
	errByServerID map[string]error
	calls         []string
}

func (f *fakeRaftPeerDemoter) DemoteRaftPeer(_ context.Context, serverID string) error {
	f.calls = append(f.calls, serverID)
	if f.errByServerID == nil {
		return nil
	}
	return f.errByServerID[serverID]
}

func TestIsBenignDemoteError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "already a non-voter",
			err:  errors.New("OpenBao API overloaded (status 500): server is already a non-voter"),
			want: true,
		},
		{
			name: "already non-voter",
			err:  errors.New("raft demote failed: already non-voter"),
			want: true,
		},
		{
			name: "non-benign error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBenignDemoteError(tt.err); got != tt.want {
				t.Fatalf("isBenignDemoteError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDemoteAllBluePods(t *testing.T) {
	t.Parallel()

	t.Run("ignores benign demote errors", func(t *testing.T) {
		t.Parallel()

		cfg := &ExecutorConfig{
			ClusterName:     "cluster",
			BlueRevision:    "blue",
			ClusterReplicas: 3,
		}
		demoter := &fakeRaftPeerDemoter{
			errByServerID: map[string]error{
				"cluster-blue-0": errors.New("server is already a non-voter"),
				"cluster-blue-1": errors.New("already non-voter"),
			},
		}

		err := demoteAllBluePods(context.Background(), logr.Discard(), cfg, demoter)
		if err != nil {
			t.Fatalf("demoteAllBluePods() unexpected error: %v", err)
		}

		if len(demoter.calls) != int(cfg.ClusterReplicas) {
			t.Fatalf("demoteAllBluePods() called DemoteRaftPeer %d times, want %d", len(demoter.calls), cfg.ClusterReplicas)
		}
	})

	t.Run("returns error on non-benign demote failure", func(t *testing.T) {
		t.Parallel()

		cfg := &ExecutorConfig{
			ClusterName:     "cluster",
			BlueRevision:    "blue",
			ClusterReplicas: 3,
		}
		demoter := &fakeRaftPeerDemoter{
			errByServerID: map[string]error{
				"cluster-blue-1": errors.New("permission denied"),
			},
		}

		err := demoteAllBluePods(context.Background(), logr.Discard(), cfg, demoter)
		if err == nil {
			t.Fatalf("demoteAllBluePods() expected error, got nil")
		}

		wantSnippet := "cluster-blue-1"
		if !strings.Contains(err.Error(), wantSnippet) {
			t.Fatalf("demoteAllBluePods() error = %q, want snippet %q", err.Error(), wantSnippet)
		}
	})
}

func TestDemoteBlueVotersExceptLeader(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "cluster",
		ClusterReplicas: 3,
	}
	bluePrefix := "cluster-blue-"
	leaderID := "cluster-blue-0"

	config := &openbao.RaftConfigurationResponse{
		Config: openbao.RaftConfiguration{
			Servers: []openbao.RaftServer{
				{NodeID: "cluster-blue-0", Address: "cluster-blue-0.cluster.ns.svc:8201", Voter: true, Leader: true},
				{NodeID: "cluster-blue-1", Address: "cluster-blue-1.cluster.ns.svc:8201", Voter: true},
				{NodeID: "cluster-blue-2", Address: "cluster-blue-2.cluster.ns.svc:8201", Voter: false},
				{NodeID: "cluster-green-0", Address: "cluster-green-0.cluster.ns.svc:8201", Voter: true},
			},
		},
	}

	demoter := &fakeRaftPeerDemoter{
		errByServerID: map[string]error{
			"cluster-blue-1": errors.New("server is already a non-voter"),
		},
	}

	demoteBlueVotersExceptLeader(context.Background(), logr.Discard(), cfg, demoter, config, leaderID, bluePrefix)

	if len(demoter.calls) != 1 {
		t.Fatalf("demoteBlueVotersExceptLeader() called DemoteRaftPeer %d times, want 1", len(demoter.calls))
	}
	if demoter.calls[0] != "cluster-blue-1" {
		t.Fatalf("demoteBlueVotersExceptLeader() called DemoteRaftPeer for %q, want %q", demoter.calls[0], "cluster-blue-1")
	}
}
