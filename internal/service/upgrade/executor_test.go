package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	openbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
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

type fakeLeaderTransferClient struct {
	readConfigFn func(context.Context) (*openbao.RaftConfigurationResponse, error)
	demoteFn     func(context.Context, string) error
	stepDownFn   func(context.Context) error
}

func (f *fakeLeaderTransferClient) ReadRaftConfiguration(ctx context.Context) (*openbao.RaftConfigurationResponse, error) {
	if f.readConfigFn != nil {
		return f.readConfigFn(ctx)
	}
	return nil, nil
}

func (f *fakeLeaderTransferClient) DemoteRaftPeer(ctx context.Context, serverID string) error {
	if f.demoteFn != nil {
		return f.demoteFn(ctx, serverID)
	}
	return nil
}

func (f *fakeLeaderTransferClient) StepDown(ctx context.Context) error {
	if f.stepDownFn != nil {
		return f.stepDownFn(ctx)
	}
	return nil
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

	err := demoteBlueVotersExceptLeader(context.Background(), logr.Discard(), cfg, demoter, config, leaderID, bluePrefix)
	if err != nil {
		t.Fatalf("demoteBlueVotersExceptLeader() unexpected error: %v", err)
	}

	if len(demoter.calls) != 1 {
		t.Fatalf("demoteBlueVotersExceptLeader() called DemoteRaftPeer %d times, want 1", len(demoter.calls))
	}
	if demoter.calls[0] != "cluster-blue-1" {
		t.Fatalf("demoteBlueVotersExceptLeader() called DemoteRaftPeer for %q, want %q", demoter.calls[0], "cluster-blue-1")
	}
}

func TestDemoteBlueVotersExceptLeaderFatal(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "cluster",
		ClusterReplicas: 3,
	}
	config := &openbao.RaftConfigurationResponse{
		Config: openbao.RaftConfiguration{
			Servers: []openbao.RaftServer{
				{NodeID: "cluster-blue-0", Voter: true, Leader: true},
				{NodeID: "cluster-blue-1", Voter: true},
			},
		},
	}
	demoter := &fakeRaftPeerDemoter{
		errByServerID: map[string]error{
			"cluster-blue-1": errors.New("permission denied"),
		},
	}

	err := demoteBlueVotersExceptLeader(
		context.Background(),
		logr.Discard(),
		cfg,
		demoter,
		config,
		"cluster-blue-0",
		"cluster-blue-",
	)
	if err == nil {
		t.Fatalf("demoteBlueVotersExceptLeader() error=nil, want fatal demote error")
	}
	if gotReason := reasonCodeFromError(err); gotReason != reasonDemoteFatal {
		t.Fatalf("demoteBlueVotersExceptLeader() reason=%q, want %q", gotReason, reasonDemoteFatal)
	}
}

func TestClassifyDemoteError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want benignErrorClassification
	}{
		{
			name: "benign already non-voter",
			err:  errors.New("already non-voter"),
			want: benignErrorClassificationBenign,
		},
		{
			name: "fatal permission denied",
			err:  errors.New("permission denied"),
			want: benignErrorClassificationFatal,
		},
		{
			name: "retryable transport failure",
			err:  errors.New("connection reset by peer"),
			want: benignErrorClassificationRetryable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyDemoteError(tt.err); got != tt.want {
				t.Fatalf("classifyDemoteError()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyStepDownError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want benignErrorClassification
	}{
		{
			name: "fatal forbidden",
			err:  errors.New("forbidden"),
			want: benignErrorClassificationFatal,
		},
		{
			name: "retryable io timeout",
			err:  errors.New("i/o timeout"),
			want: benignErrorClassificationRetryable,
		},
		{
			name: "nil",
			err:  nil,
			want: benignErrorClassificationBenign,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyStepDownError(tt.err); got != tt.want {
				t.Fatalf("classifyStepDownError()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureGreenLeaderBySteppingDownBlueWithFuncs(t *testing.T) {
	t.Parallel()

	t.Run("retries exhausted has deterministic reason", func(t *testing.T) {
		t.Parallel()

		cfg := &ExecutorConfig{
			ClusterName:     "cluster",
			BlueRevision:    "blue",
			GreenRevision:   "green",
			ClusterReplicas: 3,
		}
		blueLeaderConfig := &openbao.RaftConfigurationResponse{
			Config: openbao.RaftConfiguration{
				Servers: []openbao.RaftServer{
					{NodeID: "cluster-blue-0", Leader: true, Voter: true},
					{NodeID: "cluster-blue-1", Voter: true},
				},
			},
		}

		demoteCalls := 0
		stepDownCalls := 0
		waitCalls := 0
		fakeClient := &fakeLeaderTransferClient{
			readConfigFn: func(context.Context) (*openbao.RaftConfigurationResponse, error) {
				return blueLeaderConfig, nil
			},
			demoteFn: func(context.Context, string) error {
				demoteCalls++
				return nil
			},
			stepDownFn: func(context.Context) error {
				stepDownCalls++
				return nil
			},
		}

		resolveClient := func(context.Context, string) (leaderTransferClient, error) {
			return fakeClient, nil
		}
		waitForLeader := func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error) {
			waitCalls++
			return "https://cluster-blue-0", nil
		}

		_, err := ensureGreenLeaderBySteppingDownBlueWithFuncs(
			context.Background(),
			logr.Discard(),
			cfg,
			"https://cluster-blue-0",
			retryPolicy{MaxAttempts: 2},
			resolveClient,
			waitForLeader,
		)
		if err == nil {
			t.Fatalf("ensureGreenLeaderBySteppingDownBlueWithFuncs() error=nil, want retries exhausted")
		}
		if gotReason := reasonCodeFromError(err); gotReason != reasonLeaderTransferRetriesExhausted {
			t.Fatalf("ensureGreenLeaderBySteppingDownBlueWithFuncs() reason=%q, want %q", gotReason, reasonLeaderTransferRetriesExhausted)
		}
		if demoteCalls != 2 {
			t.Fatalf("demoteCalls=%d, want 2", demoteCalls)
		}
		if stepDownCalls != 2 {
			t.Fatalf("stepDownCalls=%d, want 2", stepDownCalls)
		}
		if waitCalls != 2 {
			t.Fatalf("waitCalls=%d, want 2", waitCalls)
		}
	})

	t.Run("fatal stepdown fails fast", func(t *testing.T) {
		t.Parallel()

		cfg := &ExecutorConfig{
			ClusterName:     "cluster",
			BlueRevision:    "blue",
			GreenRevision:   "green",
			ClusterReplicas: 3,
		}
		blueLeaderConfig := &openbao.RaftConfigurationResponse{
			Config: openbao.RaftConfiguration{
				Servers: []openbao.RaftServer{
					{NodeID: "cluster-blue-0", Leader: true, Voter: true},
					{NodeID: "cluster-blue-1", Voter: true},
				},
			},
		}

		waitCalls := 0
		fakeClient := &fakeLeaderTransferClient{
			readConfigFn: func(context.Context) (*openbao.RaftConfigurationResponse, error) {
				return blueLeaderConfig, nil
			},
			demoteFn: func(context.Context, string) error {
				return nil
			},
			stepDownFn: func(context.Context) error {
				return errors.New("permission denied")
			},
		}

		resolveClient := func(context.Context, string) (leaderTransferClient, error) {
			return fakeClient, nil
		}
		waitForLeader := func(context.Context, logr.Logger, *ExecutorConfig, string) (string, error) {
			waitCalls++
			return "", nil
		}

		_, err := ensureGreenLeaderBySteppingDownBlueWithFuncs(
			context.Background(),
			logr.Discard(),
			cfg,
			"https://cluster-blue-0",
			retryPolicy{MaxAttempts: 3},
			resolveClient,
			waitForLeader,
		)
		if err == nil {
			t.Fatalf("ensureGreenLeaderBySteppingDownBlueWithFuncs() error=nil, want stepdown fatal")
		}
		if gotReason := reasonCodeFromError(err); gotReason != reasonStepDownFatal {
			t.Fatalf("ensureGreenLeaderBySteppingDownBlueWithFuncs() reason=%q, want %q", gotReason, reasonStepDownFatal)
		}
		if waitCalls != 0 {
			t.Fatalf("waitCalls=%d, want 0", waitCalls)
		}
	})
}
