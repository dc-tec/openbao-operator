package raftops

import (
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestEvaluateGreenSyncFromAutopilot(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		GreenRevision:   "green",
		ClusterReplicas: 3,
		SyncThreshold:   5,
	}

	t.Run("all green pods present and within threshold", func(t *testing.T) {
		t.Parallel()

		state := &portopenbao.RaftAutopilotStateResponse{
			Servers: map[string]portopenbao.RaftAutopilotServerState{
				"vault-green-0": {ID: "vault-green-0", LastIndex: 100, Healthy: true},
				"vault-green-1": {ID: "vault-green-1", LastIndex: 97, Healthy: true},
				"vault-green-2": {ID: "vault-green-2", LastIndex: 100, Healthy: true},
			},
		}

		evaluation := EvaluateGreenSyncFromAutopilot(cfg, state, 100)
		if !evaluation.AllSynced {
			t.Fatalf("AllSynced = false, want true")
		}
		if evaluation.MaxDelta != 3 {
			t.Fatalf("MaxDelta = %d, want 3", evaluation.MaxDelta)
		}
		if evaluation.MissingGreen != 0 {
			t.Fatalf("MissingGreen = %d, want 0", evaluation.MissingGreen)
		}
		if evaluation.UnhealthyGreen != 0 {
			t.Fatalf("UnhealthyGreen = %d, want 0", evaluation.UnhealthyGreen)
		}
	})

	t.Run("missing unhealthy and lagging green pods block sync", func(t *testing.T) {
		t.Parallel()

		state := &portopenbao.RaftAutopilotStateResponse{
			Servers: map[string]portopenbao.RaftAutopilotServerState{
				"vault-green-0": {ID: "vault-green-0", LastIndex: 92, Healthy: true},
				"vault-green-1": {ID: "vault-green-1", LastIndex: 100, Healthy: false},
			},
		}

		evaluation := EvaluateGreenSyncFromAutopilot(cfg, state, 100)
		if evaluation.AllSynced {
			t.Fatalf("AllSynced = true, want false")
		}
		if evaluation.MaxDelta != 8 {
			t.Fatalf("MaxDelta = %d, want 8", evaluation.MaxDelta)
		}
		if evaluation.MissingGreen != 1 {
			t.Fatalf("MissingGreen = %d, want 1", evaluation.MissingGreen)
		}
		if got := evaluation.MissingPods; len(got) != 1 || got[0] != "vault-green-2" {
			t.Fatalf("MissingPods = %v, want [vault-green-2]", got)
		}
		if evaluation.UnhealthyGreen != 1 {
			t.Fatalf("UnhealthyGreen = %d, want 1", evaluation.UnhealthyGreen)
		}
		if got := evaluation.UnhealthyServers; len(got) != 1 || got[0].PodName != "vault-green-1" {
			t.Fatalf("UnhealthyServers = %+v, want vault-green-1", got)
		}
	})
}

func TestRaftAutopilotLeaderIndexHelpers(t *testing.T) {
	t.Parallel()

	t.Run("leader last index resolves by leader map key", func(t *testing.T) {
		t.Parallel()

		state := &portopenbao.RaftAutopilotStateResponse{
			Leader: "leader-key",
			Servers: map[string]portopenbao.RaftAutopilotServerState{
				"leader-key": {ID: "vault-green-0", LastIndex: 101},
				"other":      {ID: "vault-blue-0", LastIndex: 88},
			},
		}

		index, ok := RaftAutopilotLeaderLastIndex(state)
		if !ok || index != 101 {
			t.Fatalf("RaftAutopilotLeaderLastIndex() = %d, %v, want 101, true", index, ok)
		}
	})

	t.Run("leader last index falls back to status leader", func(t *testing.T) {
		t.Parallel()

		state := &portopenbao.RaftAutopilotStateResponse{
			Servers: map[string]portopenbao.RaftAutopilotServerState{
				"server-a": {ID: "vault-green-0", Status: "leader", LastIndex: 77},
				"server-b": {ID: "vault-green-1", LastIndex: 55},
			},
		}

		index, ok := RaftAutopilotLeaderLastIndex(state)
		if !ok || index != 77 {
			t.Fatalf("RaftAutopilotLeaderLastIndex() = %d, %v, want 77, true", index, ok)
		}

		maxIndex := RaftAutopilotMaxLastIndex(state)
		if maxIndex != 77 {
			t.Fatalf("RaftAutopilotMaxLastIndex() = %d, want 77", maxIndex)
		}
	})
}

func TestCountMissingGreenServers(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		GreenRevision:   "green",
		ClusterReplicas: 3,
	}

	config := &portopenbao.RaftConfigurationResponse{
		Config: portopenbao.RaftConfiguration{
			Servers: []portopenbao.RaftServer{
				{NodeID: "vault-green-0", Address: "vault-green-0.vault.default.svc"},
				{NodeID: "vault-green-2", Address: "vault-green-2.vault.default.svc"},
			},
		},
	}

	if missing := CountMissingGreenServers(cfg, config); missing != 1 {
		t.Fatalf("CountMissingGreenServers() = %d, want 1", missing)
	}
}
