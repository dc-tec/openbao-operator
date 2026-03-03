package upgrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	openbao "github.com/dc-tec/openbao-operator/internal/openbao"
)

func TestReasonCodeFromContextError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "context canceled",
			err:  context.Canceled,
			want: reasonContextCanceled,
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: reasonDeadlineExceeded,
		},
		{
			name: "non-context error",
			err:  errors.New("other"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := reasonCodeFromContextError(tt.err); got != tt.want {
				t.Fatalf("reasonCodeFromContextError()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecutorReasonedError(t *testing.T) {
	t.Parallel()

	cause := context.DeadlineExceeded
	err := newExecutorReasonedError(reasonDeadlineExceeded, "wrapped message", cause)

	if got := reasonCodeFromError(err); got != reasonDeadlineExceeded {
		t.Fatalf("reasonCodeFromError()=%q, want %q", got, reasonDeadlineExceeded)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, context.DeadlineExceeded)=false, want true")
	}
	if !strings.Contains(err.Error(), "wrapped message") {
		t.Fatalf("error text=%q, want wrapped message", err.Error())
	}
}

func TestDecisionPathFromReasonCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{
			name:   "context canceled",
			reason: reasonContextCanceled,
			want:   decisionPathContextCanceled,
		},
		{
			name:   "deadline exceeded",
			reason: reasonDeadlineExceeded,
			want:   decisionPathDeadlineExceeded,
		},
		{
			name:   "election timeout",
			reason: reasonElectionTimeout,
			want:   decisionPathElectionTimeout,
		},
		{
			name:   "unknown reason",
			reason: "reason_unknown",
			want:   decisionPathPrimaryFailedFallbackFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := decisionPathFromReasonCode(tt.reason); got != tt.want {
				t.Fatalf("decisionPathFromReasonCode()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewLeaderSearchPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		primaryRevision   string
		fallbackRevision  string
		wantAllowFallback bool
	}{
		{
			name:              "fallback enabled",
			primaryRevision:   "green",
			fallbackRevision:  "blue",
			wantAllowFallback: true,
		},
		{
			name:              "fallback disabled for empty revision",
			primaryRevision:   "green",
			fallbackRevision:  "",
			wantAllowFallback: false,
		},
		{
			name:              "fallback disabled for same revision",
			primaryRevision:   "green",
			fallbackRevision:  "green",
			wantAllowFallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := newLeaderSearchPolicy(tt.primaryRevision, tt.fallbackRevision, "primary", "fallback")
			if got.AllowFallback != tt.wantAllowFallback {
				t.Fatalf("newLeaderSearchPolicy() AllowFallback=%v, want %v", got.AllowFallback, tt.wantAllowFallback)
			}
		})
	}
}

func TestNormalizeRetryPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   retryPolicy
		want retryPolicy
	}{
		{
			name: "sets default attempts",
			in: retryPolicy{
				MaxAttempts:     0,
				AttemptInterval: 2 * time.Second,
			},
			want: retryPolicy{
				MaxAttempts:     singleLeaderSearchAttempt,
				AttemptInterval: 2 * time.Second,
			},
		},
		{
			name: "normalizes negative interval",
			in: retryPolicy{
				MaxAttempts:     3,
				AttemptInterval: -1 * time.Second,
			},
			want: retryPolicy{
				MaxAttempts:     3,
				AttemptInterval: 0,
			},
		},
		{
			name: "keeps valid values",
			in: retryPolicy{
				MaxAttempts:     4,
				AttemptInterval: 500 * time.Millisecond,
			},
			want: retryPolicy{
				MaxAttempts:     4,
				AttemptInterval: 500 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeRetryPolicy(tt.in)
			if got.MaxAttempts != tt.want.MaxAttempts {
				t.Fatalf("normalizeRetryPolicy() MaxAttempts=%d, want %d", got.MaxAttempts, tt.want.MaxAttempts)
			}
			if got.AttemptInterval != tt.want.AttemptInterval {
				t.Fatalf("normalizeRetryPolicy() AttemptInterval=%v, want %v", got.AttemptInterval, tt.want.AttemptInterval)
			}
		})
	}
}

func TestReplicaOrdinals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		replicas int32
		want     []int32
	}{
		{
			name:     "negative replicas",
			replicas: -1,
			want:     nil,
		},
		{
			name:     "zero replicas",
			replicas: 0,
			want:     nil,
		},
		{
			name:     "single replica",
			replicas: 1,
			want:     []int32{0},
		},
		{
			name:     "three replicas",
			replicas: 3,
			want:     []int32{0, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := replicaOrdinals(tt.replicas)
			if len(got) != len(tt.want) {
				t.Fatalf("len(replicaOrdinals(%d))=%d, want %d", tt.replicas, len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("replicaOrdinals(%d)[%d]=%d, want %d", tt.replicas, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAttemptOrdinals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxAttempts int
		want        []int
	}{
		{
			name:        "negative attempts",
			maxAttempts: -1,
			want:        nil,
		},
		{
			name:        "zero attempts",
			maxAttempts: 0,
			want:        nil,
		},
		{
			name:        "one attempt",
			maxAttempts: 1,
			want:        []int{0},
		},
		{
			name:        "three attempts",
			maxAttempts: 3,
			want:        []int{0, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := attemptOrdinals(tt.maxAttempts)
			if len(got) != len(tt.want) {
				t.Fatalf("len(attemptOrdinals(%d))=%d, want %d", tt.maxAttempts, len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("attemptOrdinals(%d)[%d]=%d, want %d", tt.maxAttempts, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRevisionPodName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cluster   string
		revision  string
		ordinal   int32
		wantValue string
	}{
		{
			name:      "without revision",
			cluster:   "openbao",
			revision:  "",
			ordinal:   0,
			wantValue: "openbao-0",
		},
		{
			name:      "with revision",
			cluster:   "openbao",
			revision:  "green",
			ordinal:   2,
			wantValue: "openbao-green-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := revisionPodName(tt.cluster, tt.revision, tt.ordinal); got != tt.wantValue {
				t.Fatalf("revisionPodName()=%q, want %q", got, tt.wantValue)
			}
		})
	}
}

func TestRaftServerMatchesRevision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nodeID    string
		address   string
		cluster   string
		revision  string
		replicas  int32
		wantMatch bool
	}{
		{
			name:      "matches node id",
			nodeID:    "openbao-green-1",
			cluster:   "openbao",
			revision:  "green",
			replicas:  3,
			wantMatch: true,
		},
		{
			name:      "matches address",
			address:   "https://openbao-green-2.openbao.default.svc:8201",
			cluster:   "openbao",
			revision:  "green",
			replicas:  3,
			wantMatch: true,
		},
		{
			name:      "no match",
			nodeID:    "openbao-blue-0",
			address:   "https://openbao-blue-0.openbao.default.svc:8201",
			cluster:   "openbao",
			revision:  "green",
			replicas:  3,
			wantMatch: false,
		},
		{
			name:      "no replicas",
			nodeID:    "openbao-green-0",
			cluster:   "openbao",
			revision:  "green",
			replicas:  0,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftServerMatchesRevision(tt.nodeID, tt.address, tt.cluster, tt.revision, tt.replicas); got != tt.wantMatch {
				t.Fatalf("raftServerMatchesRevision()=%v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestRaftAutopilotLeaderLastIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     *openbao.RaftAutopilotStateResponse
		wantIndex uint64
		wantFound bool
	}{
		{
			name:      "nil state",
			state:     nil,
			wantIndex: 0,
			wantFound: false,
		},
		{
			name: "leader key exists in map",
			state: &openbao.RaftAutopilotStateResponse{
				Leader: "leader-key",
				Servers: map[string]openbao.RaftAutopilotServerState{
					"leader-key": {ID: "pod-0", LastIndex: 42},
					"other":      {ID: "pod-1", LastIndex: 10},
				},
			},
			wantIndex: 42,
			wantFound: true,
		},
		{
			name: "leader resolved by server id",
			state: &openbao.RaftAutopilotStateResponse{
				Leader: "pod-0",
				Servers: map[string]openbao.RaftAutopilotServerState{
					"not-leader": {ID: "pod-0", LastIndex: 55},
				},
			},
			wantIndex: 55,
			wantFound: true,
		},
		{
			name: "leader resolved by server name",
			state: &openbao.RaftAutopilotStateResponse{
				Leader: "pod-0",
				Servers: map[string]openbao.RaftAutopilotServerState{
					"server-a": {Name: "pod-0", LastIndex: 77},
				},
			},
			wantIndex: 77,
			wantFound: true,
		},
		{
			name: "fallback to server status leader",
			state: &openbao.RaftAutopilotStateResponse{
				Leader: "unknown",
				Servers: map[string]openbao.RaftAutopilotServerState{
					"server-a": {ID: "pod-0", Status: "leader", LastIndex: 99},
				},
			},
			wantIndex: 99,
			wantFound: true,
		},
		{
			name: "leader not found",
			state: &openbao.RaftAutopilotStateResponse{
				Leader: "unknown",
				Servers: map[string]openbao.RaftAutopilotServerState{
					"server-a": {ID: "pod-0", LastIndex: 12},
				},
			},
			wantIndex: 0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotIndex, gotFound := raftAutopilotLeaderLastIndex(tt.state)
			if gotIndex != tt.wantIndex {
				t.Fatalf("raftAutopilotLeaderLastIndex() index=%d, want %d", gotIndex, tt.wantIndex)
			}
			if gotFound != tt.wantFound {
				t.Fatalf("raftAutopilotLeaderLastIndex() found=%v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func TestRaftAutopilotMaxLastIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state *openbao.RaftAutopilotStateResponse
		want  uint64
	}{
		{
			name:  "nil state",
			state: nil,
			want:  0,
		},
		{
			name: "empty servers",
			state: &openbao.RaftAutopilotStateResponse{
				Servers: map[string]openbao.RaftAutopilotServerState{},
			},
			want: 0,
		},
		{
			name: "returns max index",
			state: &openbao.RaftAutopilotStateResponse{
				Servers: map[string]openbao.RaftAutopilotServerState{
					"a": {LastIndex: 101},
					"b": {LastIndex: 88},
					"c": {LastIndex: 333},
				},
			},
			want: 333,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftAutopilotMaxLastIndex(tt.state); got != tt.want {
				t.Fatalf("raftAutopilotMaxLastIndex()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestRaftAutopilotServerMatchesPod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		server  openbao.RaftAutopilotServerState
		podName string
		want    bool
	}{
		{
			name:    "empty pod name",
			server:  openbao.RaftAutopilotServerState{ID: "pod-0"},
			podName: "",
			want:    false,
		},
		{
			name:    "matches id",
			server:  openbao.RaftAutopilotServerState{ID: "pod-0"},
			podName: "pod-0",
			want:    true,
		},
		{
			name:    "matches name",
			server:  openbao.RaftAutopilotServerState{Name: "pod-1"},
			podName: "pod-1",
			want:    true,
		},
		{
			name:    "matches address",
			server:  openbao.RaftAutopilotServerState{Address: "https://pod-2.cluster.svc:8201"},
			podName: "pod-2",
			want:    true,
		},
		{
			name:    "no match",
			server:  openbao.RaftAutopilotServerState{ID: "other"},
			podName: "pod-3",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftAutopilotServerMatchesPod(tt.server, tt.podName); got != tt.want {
				t.Fatalf("raftAutopilotServerMatchesPod()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountMissingGreenServers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cfg    *ExecutorConfig
		config *openbao.RaftConfigurationResponse
		want   int
	}{
		{
			name:   "nil config",
			cfg:    nil,
			config: nil,
			want:   0,
		},
		{
			name: "all green servers present",
			cfg: &ExecutorConfig{
				ClusterName:     "openbao",
				GreenRevision:   "green",
				ClusterReplicas: 3,
			},
			config: &openbao.RaftConfigurationResponse{
				Config: openbao.RaftConfiguration{
					Servers: []openbao.RaftServer{
						{NodeID: "openbao-green-0"},
						{NodeID: "openbao-green-1"},
						{NodeID: "openbao-green-2"},
					},
				},
			},
			want: 0,
		},
		{
			name: "counts only missing green servers",
			cfg: &ExecutorConfig{
				ClusterName:     "openbao",
				GreenRevision:   "green",
				ClusterReplicas: 4,
			},
			config: &openbao.RaftConfigurationResponse{
				Config: openbao.RaftConfiguration{
					Servers: []openbao.RaftServer{
						{NodeID: "openbao-green-0"},
						{Address: "https://openbao-green-1.openbao.default.svc:8201"},
						{NodeID: "openbao-blue-0"},
					},
				},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := countMissingGreenServers(tt.cfg, tt.config); got != tt.want {
				t.Fatalf("countMissingGreenServers()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestExecutorPodURL(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterNamespace: "default",
		ClusterName:      "openbao",
	}

	tests := []struct {
		name     string
		revision string
		ordinal  int32
		want     string
	}{
		{
			name:     "without revision",
			revision: "",
			ordinal:  0,
			want:     "https://openbao-0.openbao.default.svc:8200",
		},
		{
			name:     "with revision",
			revision: "green",
			ordinal:  2,
			want:     "https://openbao-green-2.openbao.default.svc:8200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := podURL(cfg, tt.revision, tt.ordinal); got != tt.want {
				t.Fatalf("podURL()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsBenignJoinError(t *testing.T) {
	t.Parallel()

	tests := []struct {
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
			name: "already joined",
			err:  errors.New("node already joined cluster"),
			want: true,
		},
		{
			name: "different error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isBenignJoinError(tt.err); got != tt.want {
				t.Fatalf("isBenignJoinError()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewOpenBaoClientFactory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *ExecutorConfig
		wantErr string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: "config is required",
		},
		{
			name: "valid config",
			cfg: &ExecutorConfig{
				ClusterNamespace: "default",
				ClusterName:      "openbao",
				TLSCACert:        nil,
				ClientQPS:        3.5,
				ClientBurst:      7,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			factory, cleanup, err := newOpenBaoClientFactory(tt.cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("newOpenBaoClientFactory() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("newOpenBaoClientFactory() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("newOpenBaoClientFactory() unexpected error: %v", err)
			}
			if factory == nil {
				t.Fatalf("newOpenBaoClientFactory() returned nil factory")
			}
			if cleanup == nil {
				t.Fatalf("newOpenBaoClientFactory() returned nil cleanup")
			}

			client, err := factory.New("https://openbao-0.openbao.default.svc:8200")
			if err != nil {
				t.Fatalf("factory.New() unexpected error: %v", err)
			}
			if client == nil {
				t.Fatalf("factory.New() returned nil client")
			}

			cleanup()
		})
	}
}
