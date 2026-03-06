package openbao

import (
	"context"
	"io"
)

// ClusterActions defines the contract for performing cluster-level OpenBao operations.
type ClusterActions interface {
	IsSealed(ctx context.Context) (bool, error)
	IsHealthy(ctx context.Context) (bool, error)
	IsLeader(ctx context.Context) (bool, error)
	StepDownLeader(ctx context.Context) error
	Snapshot(ctx context.Context, writer io.Writer) error
	LoginJWT(ctx context.Context, role, jwtToken string) (string, int, error)
	Restore(ctx context.Context, reader io.Reader) error
}

// RaftActions extends ClusterActions with Raft-specific operations.
type RaftActions interface {
	ClusterActions
	JoinRaftCluster(ctx context.Context, leaderAPIAddr string, retry bool, nonVoter bool) error
	ReadRaftConfiguration(ctx context.Context) (*RaftConfigurationResponse, error)
	RemoveRaftPeer(ctx context.Context, serverID string) error
	UpdateRaftConfiguration(ctx context.Context, servers []RaftServer) error
}
