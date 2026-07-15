package raftops

import (
	"context"
	"errors"
	"strings"
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type raftPeerPromoterStub struct {
	promoteErr  error
	config      *portopenbao.RaftConfigurationResponse
	configErr   error
	readResults []raftConfigReadResult

	promoteCalls int
	readCalls    int
}

type raftConfigReadResult struct {
	config *portopenbao.RaftConfigurationResponse
	err    error
}

func (s *raftPeerPromoterStub) PromoteRaftPeer(context.Context, string) error {
	s.promoteCalls++
	return s.promoteErr
}

func (s *raftPeerPromoterStub) ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	index := s.readCalls
	s.readCalls++
	if index < len(s.readResults) {
		return s.readResults[index].config, s.readResults[index].err
	}
	return s.config, s.configErr
}

func TestPromoteRaftPeerAndVerify(t *testing.T) {
	t.Parallel()

	promoteErr := errors.New("server is not a non-voter")
	verifyPolicy := RetryPolicy{MaxAttempts: 3}

	tests := []struct {
		name              string
		client            *raftPeerPromoterStub
		wantAlreadyVoter  bool
		wantErr           bool
		wantErrContains   string
		wantPromoteCalls  int
		wantReadCalls     int
		wantOriginalError bool
	}{
		{
			name: "promote success verifies raft config",
			client: &raftPeerPromoterStub{
				config: raftConfigWithServer(true),
			},
			wantPromoteCalls: 1,
			wantReadCalls:    1,
		},
		{
			name: "promote success waits for raft config to converge",
			client: &raftPeerPromoterStub{
				readResults: []raftConfigReadResult{
					{config: raftConfigWithServer(false)},
					{config: raftConfigWithServer(true)},
				},
			},
			wantPromoteCalls: 1,
			wantReadCalls:    2,
		},
		{
			name: "promote success fails when raft config does not converge",
			client: &raftPeerPromoterStub{
				config: raftConfigWithServer(false),
			},
			wantErr:          true,
			wantErrContains:  "did not become a voter after promote request",
			wantPromoteCalls: 1,
			wantReadCalls:    3,
		},
		{
			name: "promote success reports raft config verification errors",
			client: &raftPeerPromoterStub{
				configErr: errors.New("configuration unavailable"),
			},
			wantErr:          true,
			wantErrContains:  "failed to verify raft voter state after promote request",
			wantPromoteCalls: 1,
			wantReadCalls:    3,
		},
		{
			name: "promote error is benign when raft config confirms voter",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				config:     raftConfigWithServer(true),
			},
			wantAlreadyVoter: true,
			wantPromoteCalls: 1,
			wantReadCalls:    1,
		},
		{
			name: "promote already voter error is benign when raft config confirms voter",
			client: &raftPeerPromoterStub{
				promoteErr: portopenbao.ErrAlreadyVoter,
				config:     raftConfigWithServer(true),
			},
			wantAlreadyVoter: true,
			wantPromoteCalls: 1,
			wantReadCalls:    1,
		},
		{
			name: "promote already voter error waits for raft config to converge",
			client: &raftPeerPromoterStub{
				promoteErr: portopenbao.ErrAlreadyVoter,
				readResults: []raftConfigReadResult{
					{config: raftConfigWithServer(false)},
					{config: raftConfigWithServer(true)},
				},
			},
			wantAlreadyVoter: true,
			wantPromoteCalls: 1,
			wantReadCalls:    2,
		},
		{
			name: "promote already voter error is returned when raft config does not confirm voter",
			client: &raftPeerPromoterStub{
				promoteErr: portopenbao.ErrAlreadyVoter,
				config:     raftConfigWithServer(false),
			},
			wantErr:           true,
			wantPromoteCalls:  1,
			wantReadCalls:     3,
			wantOriginalError: false,
		},
		{
			name: "promote error is benign when raft config converges to voter after stale read",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				readResults: []raftConfigReadResult{
					{config: raftConfigWithServer(false)},
					{config: raftConfigWithServer(true)},
				},
			},
			wantAlreadyVoter: true,
			wantPromoteCalls: 1,
			wantReadCalls:    2,
		},
		{
			name: "promote error is benign when raft config read recovers and confirms voter",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				readResults: []raftConfigReadResult{
					{err: errors.New("configuration unavailable")},
					{config: raftConfigWithServer(true)},
				},
			},
			wantAlreadyVoter: true,
			wantPromoteCalls: 1,
			wantReadCalls:    2,
		},
		{
			name: "promote error is returned when raft config does not confirm voter",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				config:     raftConfigWithServer(false),
			},
			wantErr:           true,
			wantPromoteCalls:  1,
			wantReadCalls:     3,
			wantOriginalError: true,
		},
		{
			name: "promote error is returned when raft config read recovers but does not confirm voter",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				readResults: []raftConfigReadResult{
					{err: errors.New("configuration unavailable")},
					{config: raftConfigWithServer(false)},
				},
			},
			wantErr:           true,
			wantPromoteCalls:  1,
			wantReadCalls:     3,
			wantOriginalError: true,
		},
		{
			name: "verification error preserves promote failure context",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				configErr:  errors.New("configuration unavailable"),
			},
			wantErr:           true,
			wantErrContains:   "failed to verify raft voter state after promote error",
			wantPromoteCalls:  1,
			wantReadCalls:     3,
			wantOriginalError: true,
		},
		{
			name: "last verification error is returned when retries are exhausted",
			client: &raftPeerPromoterStub{
				promoteErr: promoteErr,
				readResults: []raftConfigReadResult{
					{err: errors.New("first read failed")},
					{err: errors.New("middle read failed")},
					{err: errors.New("last read failed")},
				},
			},
			wantErr:           true,
			wantErrContains:   "last read failed",
			wantPromoteCalls:  1,
			wantReadCalls:     3,
			wantOriginalError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			alreadyVoter, err := promoteRaftPeerAndVerifyWithPolicy(context.Background(), tt.client, "node-1", verifyPolicy)

			if alreadyVoter != tt.wantAlreadyVoter {
				t.Fatalf("alreadyVoter = %t, want %t", alreadyVoter, tt.wantAlreadyVoter)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErrContains != "" {
				gotErr := "<nil>"
				if err != nil {
					gotErr = err.Error()
				}
				if !strings.Contains(gotErr, tt.wantErrContains) {
					t.Fatalf("error = %q, want contains %q", gotErr, tt.wantErrContains)
				}
			}
			if tt.wantOriginalError && !errors.Is(err, promoteErr) {
				t.Fatalf("error = %v, want original promote error", err)
			}
			if tt.client.promoteCalls != tt.wantPromoteCalls {
				t.Fatalf("promoteCalls = %d, want %d", tt.client.promoteCalls, tt.wantPromoteCalls)
			}
			if tt.client.readCalls != tt.wantReadCalls {
				t.Fatalf("readCalls = %d, want %d", tt.client.readCalls, tt.wantReadCalls)
			}
		})
	}
}

func raftConfigWithServer(voter bool) *portopenbao.RaftConfigurationResponse {
	return &portopenbao.RaftConfigurationResponse{
		Config: portopenbao.RaftConfiguration{
			Servers: []portopenbao.RaftServer{
				{
					NodeID: "node-1",
					Voter:  voter,
				},
			},
		},
	}
}
