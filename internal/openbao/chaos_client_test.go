package openbao

import (
	"context"
	"sync/atomic"
	"testing"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

func TestChaosClient_InsertsTransientFailures(t *testing.T) {
	var calls int32
	inner := &MockClusterActions{
		IsLeaderFunc: func(ctx context.Context) (bool, error) {
			atomic.AddInt32(&calls, 1)
			return true, nil
		},
	}

	chaos := NewChaosClient(inner, ChaosConfig{
		FailProbability: 1.0,
		Seed:            1,
	})

	_, err := chaos.IsLeader(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if !operatorerrors.IsTransientConnection(err) {
		t.Fatalf("expected transient connection error, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected inner client not called, got %d", got)
	}
}
