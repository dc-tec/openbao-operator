package upgrade

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

type ordinalNumber interface {
	~int | ~int32
}

type ordinalTestCase[N ordinalNumber] struct {
	name  string
	input N
	want  []N
}

func ordinalCases[N ordinalNumber]() []ordinalTestCase[N] {
	return []ordinalTestCase[N]{
		{name: "negative input", input: -1, want: nil},
		{name: "zero input", input: 0, want: nil},
		{name: "single value", input: 1, want: []N{0}},
		{name: "three values", input: 3, want: []N{0, 1, 2}},
	}
}

func runOrdinalTests[N ordinalNumber](t *testing.T, name string, fn func(N) []N, tests []ordinalTestCase[N]) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fn(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len(%s(%d))=%d, want %d", name, tt.input, len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("%s(%d)[%d]=%d, want %d", name, tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

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
			want: raftops.ReasonContextCanceled,
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: raftops.ReasonDeadlineExceeded,
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
			if got := raftops.ReasonCodeFromContextError(tt.err); got != tt.want {
				t.Fatalf("raftops.ReasonCodeFromContextError()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecutorReasonedError(t *testing.T) {
	t.Parallel()

	cause := context.DeadlineExceeded
	err := raftops.NewExecutorReasonedError(raftops.ReasonDeadlineExceeded, "wrapped message", cause)

	if got := raftops.ReasonCodeFromError(err); got != raftops.ReasonDeadlineExceeded {
		t.Fatalf("raftops.ReasonCodeFromError()=%q, want %q", got, raftops.ReasonDeadlineExceeded)
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
			reason: raftops.ReasonContextCanceled,
			want:   raftops.DecisionPathContextCanceled,
		},
		{
			name:   "deadline exceeded",
			reason: raftops.ReasonDeadlineExceeded,
			want:   raftops.DecisionPathDeadlineExceeded,
		},
		{
			name:   "election timeout",
			reason: raftops.ReasonElectionTimeout,
			want:   raftops.DecisionPathElectionTimeout,
		},
		{
			name:   "unknown reason",
			reason: "reason_unknown",
			want:   raftops.DecisionPathPrimaryFailedFallbackFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftops.DecisionPathFromReasonCode(tt.reason); got != tt.want {
				t.Fatalf("raftops.DecisionPathFromReasonCode()=%q, want %q", got, tt.want)
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

			got := raftops.NewLeaderSearchPolicy(tt.primaryRevision, tt.fallbackRevision, "primary", "fallback")
			if got.AllowFallback != tt.wantAllowFallback {
				t.Fatalf("raftops.NewLeaderSearchPolicy() AllowFallback=%v, want %v", got.AllowFallback, tt.wantAllowFallback)
			}
		})
	}
}

func TestNormalizeRetryPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   raftops.RetryPolicy
		want raftops.RetryPolicy
	}{
		{
			name: "sets default attempts",
			in: raftops.RetryPolicy{
				MaxAttempts:     0,
				AttemptInterval: 2 * time.Second,
			},
			want: raftops.RetryPolicy{
				MaxAttempts:     1,
				AttemptInterval: 2 * time.Second,
			},
		},
		{
			name: "normalizes negative interval",
			in: raftops.RetryPolicy{
				MaxAttempts:     3,
				AttemptInterval: -1 * time.Second,
			},
			want: raftops.RetryPolicy{
				MaxAttempts:     3,
				AttemptInterval: 0,
			},
		},
		{
			name: "keeps valid values",
			in: raftops.RetryPolicy{
				MaxAttempts:     4,
				AttemptInterval: 500 * time.Millisecond,
			},
			want: raftops.RetryPolicy{
				MaxAttempts:     4,
				AttemptInterval: 500 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := raftops.NormalizeRetryPolicy(tt.in)
			if got.MaxAttempts != tt.want.MaxAttempts {
				t.Fatalf("normalizeRetryPolicy() MaxAttempts=%d, want %d", got.MaxAttempts, tt.want.MaxAttempts)
			}
			if got.AttemptInterval != tt.want.AttemptInterval {
				t.Fatalf("normalizeRetryPolicy() AttemptInterval=%v, want %v", got.AttemptInterval, tt.want.AttemptInterval)
			}
		})
	}
}

func TestMaxLeaderSearchAttempts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy raftops.LeaderSearchPolicy
		want   int
	}{
		{
			name: "fallback enabled",
			policy: raftops.LeaderSearchPolicy{
				AllowFallback: true,
			},
			want: 2,
		},
		{
			name: "fallback disabled",
			policy: raftops.LeaderSearchPolicy{
				AllowFallback: false,
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftops.MaxLeaderSearchAttempts(tt.policy); got != tt.want {
				t.Fatalf("raftops.MaxLeaderSearchAttempts()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestReplicaOrdinals(t *testing.T) {
	t.Parallel()

	runOrdinalTests(t, "raftops.ReplicaOrdinals", raftops.ReplicaOrdinals, ordinalCases[int32]())
}

func TestAttemptOrdinals(t *testing.T) {
	t.Parallel()

	runOrdinalTests(t, "raftops.AttemptOrdinals", raftops.AttemptOrdinals, ordinalCases[int]())
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
			if got := raftops.RevisionPodName(tt.cluster, tt.revision, tt.ordinal); got != tt.wantValue {
				t.Fatalf("raftops.RevisionPodName()=%q, want %q", got, tt.wantValue)
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
			if got := raftops.RaftServerMatchesRevision(tt.nodeID, tt.address, tt.cluster, tt.revision, tt.replicas); got != tt.wantMatch {
				t.Fatalf("raftops.RaftServerMatchesRevision()=%v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestRaftAutopilotLeaderLastIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     *portopenbao.RaftAutopilotStateResponse
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
			state: &portopenbao.RaftAutopilotStateResponse{
				Leader: "leader-key",
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"leader-key": {ID: "pod-0", LastIndex: 42},
					"other":      {ID: "pod-1", LastIndex: 10},
				},
			},
			wantIndex: 42,
			wantFound: true,
		},
		{
			name: "leader resolved by server id",
			state: &portopenbao.RaftAutopilotStateResponse{
				Leader: "pod-0",
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"not-leader": {ID: "pod-0", LastIndex: 55},
				},
			},
			wantIndex: 55,
			wantFound: true,
		},
		{
			name: "leader resolved by server name",
			state: &portopenbao.RaftAutopilotStateResponse{
				Leader: "pod-0",
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"server-a": {Name: "pod-0", LastIndex: 77},
				},
			},
			wantIndex: 77,
			wantFound: true,
		},
		{
			name: "fallback to server status leader",
			state: &portopenbao.RaftAutopilotStateResponse{
				Leader: "unknown",
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"server-a": {ID: "pod-0", Status: "leader", LastIndex: 99},
				},
			},
			wantIndex: 99,
			wantFound: true,
		},
		{
			name: "leader not found",
			state: &portopenbao.RaftAutopilotStateResponse{
				Leader: "unknown",
				Servers: map[string]portopenbao.RaftAutopilotServerState{
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
			gotIndex, gotFound := raftops.RaftAutopilotLeaderLastIndex(tt.state)
			if gotIndex != tt.wantIndex {
				t.Fatalf("raftops.RaftAutopilotLeaderLastIndex() index=%d, want %d", gotIndex, tt.wantIndex)
			}
			if gotFound != tt.wantFound {
				t.Fatalf("raftops.RaftAutopilotLeaderLastIndex() found=%v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func TestRaftAutopilotMaxLastIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state *portopenbao.RaftAutopilotStateResponse
		want  uint64
	}{
		{
			name:  "nil state",
			state: nil,
			want:  0,
		},
		{
			name: "empty servers",
			state: &portopenbao.RaftAutopilotStateResponse{
				Servers: map[string]portopenbao.RaftAutopilotServerState{},
			},
			want: 0,
		},
		{
			name: "returns max index",
			state: &portopenbao.RaftAutopilotStateResponse{
				Servers: map[string]portopenbao.RaftAutopilotServerState{
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
			if got := raftops.RaftAutopilotMaxLastIndex(tt.state); got != tt.want {
				t.Fatalf("raftops.RaftAutopilotMaxLastIndex()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestRaftAutopilotServerMatchesPod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		server  portopenbao.RaftAutopilotServerState
		podName string
		want    bool
	}{
		{
			name:    "empty pod name",
			server:  portopenbao.RaftAutopilotServerState{ID: "pod-0"},
			podName: "",
			want:    false,
		},
		{
			name:    "matches id",
			server:  portopenbao.RaftAutopilotServerState{ID: "pod-0"},
			podName: "pod-0",
			want:    true,
		},
		{
			name:    "matches name",
			server:  portopenbao.RaftAutopilotServerState{Name: "pod-1"},
			podName: "pod-1",
			want:    true,
		},
		{
			name:    "matches address",
			server:  portopenbao.RaftAutopilotServerState{Address: "https://pod-2.cluster.svc:8201"},
			podName: "pod-2",
			want:    true,
		},
		{
			name:    "no match",
			server:  portopenbao.RaftAutopilotServerState{ID: "other"},
			podName: "pod-3",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftops.RaftAutopilotServerMatchesPod(tt.server, tt.podName); got != tt.want {
				t.Fatalf("raftops.RaftAutopilotServerMatchesPod()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateGreenSyncFromAutopilot(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "openbao",
		GreenRevision:   "green",
		ClusterReplicas: 2,
		SyncThreshold:   10,
	}

	tests := []struct {
		name               string
		state              *portopenbao.RaftAutopilotStateResponse
		targetIndex        uint64
		wantAllSynced      bool
		wantMaxDelta       uint64
		wantMissingGreen   int
		wantUnhealthyGreen int
		wantMissingPods    []string
	}{
		{
			name: "all green pods synced",
			state: &portopenbao.RaftAutopilotStateResponse{
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"a": {ID: "openbao-green-0", LastIndex: 100, Healthy: true},
					"b": {ID: "openbao-green-1", LastIndex: 95, Healthy: true},
				},
			},
			targetIndex:        100,
			wantAllSynced:      true,
			wantMaxDelta:       5,
			wantMissingGreen:   0,
			wantUnhealthyGreen: 0,
		},
		{
			name: "missing green pod blocks sync",
			state: &portopenbao.RaftAutopilotStateResponse{
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"a": {ID: "openbao-green-0", LastIndex: 100, Healthy: true},
				},
			},
			targetIndex:        100,
			wantAllSynced:      false,
			wantMaxDelta:       0,
			wantMissingGreen:   1,
			wantUnhealthyGreen: 0,
			wantMissingPods:    []string{"openbao-green-1"},
		},
		{
			name: "delta above threshold blocks sync",
			state: &portopenbao.RaftAutopilotStateResponse{
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"a": {ID: "openbao-green-0", LastIndex: 100, Healthy: true},
					"b": {ID: "openbao-green-1", LastIndex: 80, Healthy: true},
				},
			},
			targetIndex:        100,
			wantAllSynced:      false,
			wantMaxDelta:       20,
			wantMissingGreen:   0,
			wantUnhealthyGreen: 0,
		},
		{
			name: "unhealthy green is tracked but not blocking by itself",
			state: &portopenbao.RaftAutopilotStateResponse{
				Servers: map[string]portopenbao.RaftAutopilotServerState{
					"a": {ID: "openbao-green-0", LastIndex: 100, Healthy: false, Status: "follower"},
					"b": {ID: "openbao-green-1", LastIndex: 100, Healthy: true},
				},
			},
			targetIndex:        100,
			wantAllSynced:      true,
			wantMaxDelta:       0,
			wantMissingGreen:   0,
			wantUnhealthyGreen: 1,
		},
		{
			name:               "nil state is unsynced",
			state:              nil,
			targetIndex:        100,
			wantAllSynced:      false,
			wantMaxDelta:       0,
			wantMissingGreen:   0,
			wantUnhealthyGreen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := raftops.EvaluateGreenSyncFromAutopilot(cfg, tt.state, tt.targetIndex)
			if got.AllSynced != tt.wantAllSynced {
				t.Fatalf("AllSynced=%v, want %v", got.AllSynced, tt.wantAllSynced)
			}
			if got.MaxDelta != tt.wantMaxDelta {
				t.Fatalf("MaxDelta=%d, want %d", got.MaxDelta, tt.wantMaxDelta)
			}
			if got.MissingGreen != tt.wantMissingGreen {
				t.Fatalf("MissingGreen=%d, want %d", got.MissingGreen, tt.wantMissingGreen)
			}
			if got.UnhealthyGreen != tt.wantUnhealthyGreen {
				t.Fatalf("UnhealthyGreen=%d, want %d", got.UnhealthyGreen, tt.wantUnhealthyGreen)
			}
			if len(got.MissingPods) != len(tt.wantMissingPods) {
				t.Fatalf("len(MissingPods)=%d, want %d", len(got.MissingPods), len(tt.wantMissingPods))
			}
			for i := range tt.wantMissingPods {
				if got.MissingPods[i] != tt.wantMissingPods[i] {
					t.Fatalf("MissingPods[%d]=%q, want %q", i, got.MissingPods[i], tt.wantMissingPods[i])
				}
			}
		})
	}
}

func TestFindAutopilotServerForPod(t *testing.T) {
	t.Parallel()

	state := &portopenbao.RaftAutopilotStateResponse{
		Servers: map[string]portopenbao.RaftAutopilotServerState{
			"a": {ID: "openbao-green-0", LastIndex: 10},
		},
	}

	server, found := raftops.FindAutopilotServerForPod(state, "openbao-green-0")
	if !found {
		t.Fatalf("raftops.FindAutopilotServerForPod() found=false, want true")
	}
	if server.ID != "openbao-green-0" {
		t.Fatalf("raftops.FindAutopilotServerForPod() server.ID=%q, want %q", server.ID, "openbao-green-0")
	}

	_, found = raftops.FindAutopilotServerForPod(state, "openbao-green-1")
	if found {
		t.Fatalf("raftops.FindAutopilotServerForPod() found=true for missing pod, want false")
	}
}

func TestAutopilotServerDebugNames(t *testing.T) {
	t.Parallel()

	state := &portopenbao.RaftAutopilotStateResponse{
		Servers: map[string]portopenbao.RaftAutopilotServerState{
			"z": {ID: "id-z", Name: "pod-z", Address: "https://pod-z"},
			"a": {ID: "id-a", Name: "pod-a", Address: "https://pod-a"},
		},
	}

	got := raftops.AutopilotServerDebugNames(state)
	want := []string{
		"a(id=id-a,name=pod-a,addr=https://pod-a)",
		"z(id=id-z,name=pod-z,addr=https://pod-z)",
	}
	if len(got) != len(want) {
		t.Fatalf("len(raftops.AutopilotServerDebugNames)=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("raftops.AutopilotServerDebugNames[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestCountMissingGreenServers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cfg    *ExecutorConfig
		config *portopenbao.RaftConfigurationResponse
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
			config: &portopenbao.RaftConfigurationResponse{
				Config: portopenbao.RaftConfiguration{
					Servers: []portopenbao.RaftServer{
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
			config: &portopenbao.RaftConfigurationResponse{
				Config: portopenbao.RaftConfiguration{
					Servers: []portopenbao.RaftServer{
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
			if got := raftops.CountMissingGreenServers(tt.cfg, tt.config); got != tt.want {
				t.Fatalf("raftops.CountMissingGreenServers()=%d, want %d", got, tt.want)
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
			if got := raftops.PodURL(cfg, tt.revision, tt.ordinal); got != tt.want {
				t.Fatalf("raftops.PodURL()=%q, want %q", got, tt.want)
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
			err:  portopenbao.ErrAlreadyJoined,
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
			if got := raftops.IsBenignJoinError(tt.err); got != tt.want {
				t.Fatalf("raftops.IsBenignJoinError()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyJoinError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want raftops.BenignErrorClassification
	}{
		{
			name: "nil",
			err:  nil,
			want: raftops.BenignErrorClassificationBenign,
		},
		{
			name: "already joined is benign",
			err:  portopenbao.ErrAlreadyJoined,
			want: raftops.BenignErrorClassificationBenign,
		},
		{
			name: "permission denied is fatal",
			err:  portopenbao.NewAPIError("raft join request failed", http.StatusForbidden, nil),
			want: raftops.BenignErrorClassificationFatal,
		},
		{
			name: "unknown defaults to fatal",
			err:  errors.New("some other join error"),
			want: raftops.BenignErrorClassificationFatal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := raftops.ClassifyJoinError(tt.err); got != tt.want {
				t.Fatalf("raftops.ClassifyJoinError()=%q, want %q", got, tt.want)
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

			factory, cleanup, err := raftops.NewOpenBaoClientFactory(tt.cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("raftops.NewOpenBaoClientFactory() error=nil, want contains %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("raftops.NewOpenBaoClientFactory() error=%q, want contains %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("raftops.NewOpenBaoClientFactory() unexpected error: %v", err)
			}
			if factory == nil {
				t.Fatalf("raftops.NewOpenBaoClientFactory() returned nil factory")
			}
			if cleanup == nil {
				t.Fatalf("raftops.NewOpenBaoClientFactory() returned nil cleanup")
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
