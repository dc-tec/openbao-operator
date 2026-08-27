package openbao

import (
	"context"
	"io"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// MockClusterActions is a test implementation of the OpenBao ClusterActions port.
type MockClusterActions struct {
	IsSealedFunc       func(ctx context.Context) (bool, error)
	IsHealthyFunc      func(ctx context.Context) (bool, error)
	IsLeaderFunc       func(ctx context.Context) (bool, error)
	StepDownLeaderFunc func(ctx context.Context) error
	SnapshotFunc       func(ctx context.Context, writer io.Writer) error
	LoginJWTFunc       func(ctx context.Context, role, jwtToken string) (string, int, error)
	RestoreFunc        func(ctx context.Context, reader io.Reader, options portopenbao.RestoreOptions) error
}

func (m *MockClusterActions) IsSealed(ctx context.Context) (bool, error) {
	if m.IsSealedFunc != nil {
		return m.IsSealedFunc(ctx)
	}
	return false, nil
}

func (m *MockClusterActions) IsHealthy(ctx context.Context) (bool, error) {
	if m.IsHealthyFunc != nil {
		return m.IsHealthyFunc(ctx)
	}
	return true, nil
}

func (m *MockClusterActions) IsLeader(ctx context.Context) (bool, error) {
	if m.IsLeaderFunc != nil {
		return m.IsLeaderFunc(ctx)
	}
	return false, nil
}

func (m *MockClusterActions) StepDownLeader(ctx context.Context) error {
	if m.StepDownLeaderFunc != nil {
		return m.StepDownLeaderFunc(ctx)
	}
	return nil
}

func (m *MockClusterActions) Snapshot(ctx context.Context, writer io.Writer) error {
	if m.SnapshotFunc != nil {
		return m.SnapshotFunc(ctx, writer)
	}
	return nil
}

func (m *MockClusterActions) LoginJWT(ctx context.Context, role, jwtToken string) (string, int, error) {
	if m.LoginJWTFunc != nil {
		return m.LoginJWTFunc(ctx, role, jwtToken)
	}
	return "mock-token", 0, nil
}

func (m *MockClusterActions) Restore(ctx context.Context, reader io.Reader, options portopenbao.RestoreOptions) error {
	if m.RestoreFunc != nil {
		return m.RestoreFunc(ctx, reader, options)
	}
	return nil
}
