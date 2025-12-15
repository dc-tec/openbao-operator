package openbao

import (
	"context"
	"io"
)

// ClusterActions defines the interface for performing cluster-level operations on OpenBao.
// This interface describes intent rather than specific API calls, allowing for easier
// testing and potential future implementations.
//
// Implementations of this interface should handle the underlying HTTP communication
// and error handling, providing a clean abstraction for cluster management operations.
type ClusterActions interface {
	// IsSealed checks if the OpenBao cluster is sealed.
	// Returns true if sealed, false if unsealed, and an error if the check fails.
	IsSealed(ctx context.Context) (bool, error)

	// IsHealthy checks if the OpenBao cluster is healthy (initialized and unsealed).
	// Returns true if healthy, false otherwise, and an error if the check fails.
	IsHealthy(ctx context.Context) (bool, error)

	// IsLeader checks if the connected node is the Raft leader.
	// Returns true if this node is the leader, false otherwise, and an error if the check fails.
	IsLeader(ctx context.Context) (bool, error)

	// StepDownLeader requests the leader to step down and trigger a new election.
	// This is used during upgrades to gracefully transfer leadership before updating the leader pod.
	// Returns an error if the step-down operation fails.
	StepDownLeader(ctx context.Context) error

	// Snapshot retrieves a Raft snapshot from the leader and writes it to the provided writer.
	// The caller is responsible for closing any resources returned by the implementation.
	// Returns an error if the snapshot operation fails.
	Snapshot(ctx context.Context, writer io.Writer) error

	// LoginJWT authenticates to OpenBao using JWT authentication.
	// Returns the OpenBao client token on success, or an error if authentication fails.
	LoginJWT(ctx context.Context, role, jwtToken string) (string, error)
}
