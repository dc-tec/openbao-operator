package raftops

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type consensusRepairStub struct {
	config *portopenbao.RaftConfigurationResponse

	promoted []string
	demoted  []string
}

func (s *consensusRepairStub) ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	return s.config, nil
}

func (s *consensusRepairStub) PromoteRaftPeer(_ context.Context, serverID string) error {
	s.promoted = append(s.promoted, serverID)
	for i := range s.config.Config.Servers {
		if s.config.Config.Servers[i].NodeID == serverID {
			s.config.Config.Servers[i].Voter = true
		}
	}
	return nil
}

func (s *consensusRepairStub) DemoteRaftPeer(_ context.Context, serverID string) error {
	s.demoted = append(s.demoted, serverID)
	return nil
}

func repairTestServers(cfg *ExecutorConfig, revision string, voter bool, ordinals ...int32) []portopenbao.RaftServer {
	servers := make([]portopenbao.RaftServer, 0, len(ordinals))
	for _, ordinal := range ordinals {
		servers = append(servers, portopenbao.RaftServer{
			NodeID: RevisionPodName(cfg.ClusterName, revision, ordinal),
			Voter:  voter,
		})
	}
	return servers
}

func TestRepairBlueGreenConsensus_BlueQuorumGuard(t *testing.T) {
	t.Parallel()

	cfg := &ExecutorConfig{
		ClusterName:     "vault",
		ClusterReplicas: 3,
		BlueRevision:    "blue",
		GreenRevision:   "green",
	}

	tests := []struct {
		name          string
		blueOrdinals  []int32
		wantErr       string
		wantPromoted  int
		wantDemotions int
	}{
		{
			name:          "all Blue peers present",
			blueOrdinals:  []int32{0, 1, 2},
			wantPromoted:  3,
			wantDemotions: 3,
		},
		{
			name:          "Blue quorum present",
			blueOrdinals:  []int32{0, 1},
			wantPromoted:  2,
			wantDemotions: 3,
		},
		{
			name:         "Blue below quorum",
			blueOrdinals: []int32{0},
			wantErr:      "1 Blue peers remain in the Raft configuration, need 2 of 3",
		},
		{
			name:         "Blue peers removed",
			blueOrdinals: nil,
			wantErr:      "0 Blue peers remain in the Raft configuration, need 2 of 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			servers := repairTestServers(cfg, cfg.BlueRevision, false, tt.blueOrdinals...)
			servers = append(servers, repairTestServers(cfg, cfg.GreenRevision, true, 0, 1, 2)...)
			client := &consensusRepairStub{
				config: &portopenbao.RaftConfigurationResponse{Config: portopenbao.RaftConfiguration{Servers: servers}},
			}

			err := repairBlueGreenConsensus(context.Background(), logr.Discard(), cfg, client, RetryPolicy{MaxAttempts: 1})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("repairBlueGreenConsensus() error = %v, want containing %q", err, tt.wantErr)
				}
				if len(client.promoted) != 0 || len(client.demoted) != 0 {
					t.Fatalf("membership changed after refusal: promoted=%v demoted=%v", client.promoted, client.demoted)
				}
				return
			}
			if err != nil {
				t.Fatalf("repairBlueGreenConsensus() error = %v", err)
			}
			if len(client.promoted) != tt.wantPromoted {
				t.Fatalf("promoted = %v, want %d Blue peers", client.promoted, tt.wantPromoted)
			}
			if len(client.demoted) != tt.wantDemotions {
				t.Fatalf("demoted = %v, want %d Green peers", client.demoted, tt.wantDemotions)
			}
		})
	}
}
