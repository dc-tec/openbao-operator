package openbao

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
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
	LoginJWTFunc func(ctx context.Context, role, jwtToken string) (string, int, error)
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
func (m *MockClusterActions) LoginJWT(ctx context.Context, role, jwtToken string) (string, int, error) {
	if m.LoginJWTFunc != nil {
		return m.LoginJWTFunc(ctx, role, jwtToken)
	}
	return "mock-token", 0, nil
}

// Restore implements ClusterActions.
func (m *MockClusterActions) Restore(ctx context.Context, reader io.Reader) error {
	if m.RestoreFunc != nil {
		return m.RestoreFunc(ctx, reader)
	}
	return nil
}

// ChaosConfig controls error injection for tests.
// It is intended for unit/integration tests only, not production usage.
type ChaosConfig struct {
	// FailProbability is the probability (0.0-1.0) of injecting a transient connection error.
	FailProbability float64
	// OverloadProbability is the probability (0.0-1.0) of injecting a transient remote overloaded error.
	OverloadProbability float64
	// Seed controls RNG determinism. If zero, time.Now is used.
	Seed int64
}

// ChaosClient wraps ClusterActions and injects transient failures.
type ChaosClient struct {
	inner ClusterActions
	cfg   ChaosConfig

	mu  sync.Mutex
	rng *rand.Rand
}

func NewChaosClient(inner ClusterActions, cfg ChaosConfig) *ChaosClient {
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &ChaosClient{
		inner: inner,
		cfg:   cfg,
		// #nosec G404 -- This RNG is for deterministic chaos injection in unit/integration tests,
		// not for security-sensitive randomness.
		rng: rand.New(rand.NewSource(seed)),
	}
}

func (c *ChaosClient) maybeFail() error {
	if c == nil || c.inner == nil {
		return fmt.Errorf("chaos client requires inner client")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	roll := c.rng.Float64()
	if c.cfg.FailProbability > 0 && roll < c.cfg.FailProbability {
		return operatorerrors.WrapTransientConnection(fmt.Errorf("chaos: injected transient connection failure"))
	}

	if c.cfg.OverloadProbability > 0 && roll < c.cfg.FailProbability+c.cfg.OverloadProbability {
		return operatorerrors.WrapTransientRemoteOverloaded(fmt.Errorf("chaos: injected remote overloaded failure"))
	}

	return nil
}

func (c *ChaosClient) IsSealed(ctx context.Context) (bool, error) {
	if err := c.maybeFail(); err != nil {
		return false, err
	}
	return c.inner.IsSealed(ctx)
}

func (c *ChaosClient) IsHealthy(ctx context.Context) (bool, error) {
	if err := c.maybeFail(); err != nil {
		return false, err
	}
	return c.inner.IsHealthy(ctx)
}

func (c *ChaosClient) IsLeader(ctx context.Context) (bool, error) {
	if err := c.maybeFail(); err != nil {
		return false, err
	}
	return c.inner.IsLeader(ctx)
}

func (c *ChaosClient) StepDownLeader(ctx context.Context) error {
	if err := c.maybeFail(); err != nil {
		return err
	}
	return c.inner.StepDownLeader(ctx)
}

func (c *ChaosClient) Snapshot(ctx context.Context, writer io.Writer) error {
	if err := c.maybeFail(); err != nil {
		return err
	}
	return c.inner.Snapshot(ctx, writer)
}

func (c *ChaosClient) LoginJWT(ctx context.Context, role, jwtToken string) (string, int, error) {
	if err := c.maybeFail(); err != nil {
		return "", 0, err
	}
	return c.inner.LoginJWT(ctx, role, jwtToken)
}

func (c *ChaosClient) Restore(ctx context.Context, reader io.Reader) error {
	if err := c.maybeFail(); err != nil {
		return err
	}
	return c.inner.Restore(ctx, reader)
}
