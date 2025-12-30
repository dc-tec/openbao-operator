package openbao

import (
	"context"
	"io"
)

// MockClusterActions is a mock implementation of ClusterActions for testing.
// It allows tests to control the behavior of OpenBao client operations without
// requiring actual HTTP servers or network connections.
type MockClusterActions struct {
	// IsSealedFunc controls the behavior of IsSealed
	IsSealedFunc func(ctx context.Context) (bool, error)
	// IsHealthyFunc controls the behavior of IsHealthy
	IsHealthyFunc func(ctx context.Context) (bool, error)
	// IsLeaderFunc controls the behavior of IsLeader
	IsLeaderFunc func(ctx context.Context) (bool, error)
	// StepDownLeaderFunc controls the behavior of StepDownLeader
	StepDownLeaderFunc func(ctx context.Context) error
	// SnapshotFunc controls the behavior of Snapshot
	SnapshotFunc func(ctx context.Context, writer io.Writer) error
	// LoginJWTFunc controls the behavior of LoginJWT
	LoginJWTFunc func(ctx context.Context, role, jwtToken string) (string, error)
	// RestoreFunc controls the behavior of Restore
	RestoreFunc func(ctx context.Context, reader io.Reader) error
}

// IsSealed implements ClusterActions.
func (m *MockClusterActions) IsSealed(ctx context.Context) (bool, error) {
	if m.IsSealedFunc != nil {
		return m.IsSealedFunc(ctx)
	}
	return false, nil
}

// IsHealthy implements ClusterActions.
func (m *MockClusterActions) IsHealthy(ctx context.Context) (bool, error) {
	if m.IsHealthyFunc != nil {
		return m.IsHealthyFunc(ctx)
	}
	return true, nil
}

// IsLeader implements ClusterActions.
func (m *MockClusterActions) IsLeader(ctx context.Context) (bool, error) {
	if m.IsLeaderFunc != nil {
		return m.IsLeaderFunc(ctx)
	}
	return false, nil
}

// StepDownLeader implements ClusterActions.
func (m *MockClusterActions) StepDownLeader(ctx context.Context) error {
	if m.StepDownLeaderFunc != nil {
		return m.StepDownLeaderFunc(ctx)
	}
	return nil
}

// Snapshot implements ClusterActions.
func (m *MockClusterActions) Snapshot(ctx context.Context, writer io.Writer) error {
	if m.SnapshotFunc != nil {
		return m.SnapshotFunc(ctx, writer)
	}
	return nil
}

// LoginJWT implements ClusterActions.
func (m *MockClusterActions) LoginJWT(ctx context.Context, role, jwtToken string) (string, error) {
	if m.LoginJWTFunc != nil {
		return m.LoginJWTFunc(ctx, role, jwtToken)
	}
	return "mock-token", nil
}

// Restore implements ClusterActions.
func (m *MockClusterActions) Restore(ctx context.Context, reader io.Reader) error {
	if m.RestoreFunc != nil {
		return m.RestoreFunc(ctx, reader)
	}
	return nil
}
