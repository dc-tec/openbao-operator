package raftops

import (
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/go-logr/logr"
)

func TestRepairConsensusUsesCapturedBlueReplicaCount(t *testing.T) {
	cfg := &ExecutorConfig{ClusterName: "vault", ClusterReplicas: 7, BlueReplicas: 3, BlueRevision: "blue", GreenRevision: "green"}
	servers := repairTestServers(cfg, cfg.BlueRevision, true, 0, 1, 2)
	servers = append(servers, repairTestServers(cfg, cfg.GreenRevision, false, 0, 1, 2)...)
	client := &consensusRepairStub{config: &portopenbao.RaftConfigurationResponse{Config: portopenbao.RaftConfiguration{Servers: servers}}}
	if err := repairBlueGreenConsensus(t.Context(), logr.Discard(), cfg, client, RetryPolicy{MaxAttempts: 1}); err != nil {
		t.Fatalf("intact three-voter Blue cluster refused after desired count changed to seven: %v", err)
	}
}
