package openbao

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

func TestClientManager_FactoryFor_ReturnsSameStateForSameCluster(t *testing.T) {
	t.Parallel()

	mgr := NewClientManager(ClientConfig{
		RateLimitQPS:                   100,
		RateLimitBurst:                 100,
		CircuitBreakerFailureThreshold: 5,
		CircuitBreakerOpenDuration:     10 * time.Second,
	})
	defer mgr.Close()

	clusterKey := "ns/cluster1"
	caCert := []byte{} // Empty for this test

	f1 := mgr.FactoryFor(clusterKey, caCert)
	f2 := mgr.FactoryFor(clusterKey, caCert)

	if f1.clientState != f2.clientState {
		t.Errorf("expected same clientState for same clusterKey, got different pointers")
	}
}

func TestClientManager_FactoryFor_DifferentStateForDifferentClusters(t *testing.T) {
	t.Parallel()

	mgr := NewClientManager(ClientConfig{
		RateLimitQPS:   100,
		RateLimitBurst: 100,
	})
	defer mgr.Close()

	f1 := mgr.FactoryFor("ns/cluster1", nil)
	f2 := mgr.FactoryFor("ns/cluster2", nil)

	if f1.clientState == f2.clientState {
		t.Errorf("expected different clientState for different clusterKeys, got same pointer")
	}
}

func TestClientManager_ClusterCount(t *testing.T) {
	t.Parallel()

	mgr := NewClientManager(ClientConfig{})
	defer mgr.Close()

	if got := mgr.ClusterCount(); got != 0 {
		t.Errorf("ClusterCount()=%d, expected 0", got)
	}

	_ = mgr.FactoryFor("ns/cluster1", nil)
	if got := mgr.ClusterCount(); got != 1 {
		t.Errorf("ClusterCount()=%d, expected 1", got)
	}

	_ = mgr.FactoryFor("ns/cluster2", nil)
	if got := mgr.ClusterCount(); got != 2 {
		t.Errorf("ClusterCount()=%d, expected 2", got)
	}

	// Same cluster key should not increase count
	_ = mgr.FactoryFor("ns/cluster1", nil)
	if got := mgr.ClusterCount(); got != 2 {
		t.Errorf("ClusterCount()=%d, expected 2 (same cluster)", got)
	}
}

func TestClientManager_ClearCluster(t *testing.T) {
	t.Parallel()

	mgr := NewClientManager(ClientConfig{})
	defer mgr.Close()

	_ = mgr.FactoryFor("ns/cluster1", nil)
	_ = mgr.FactoryFor("ns/cluster2", nil)

	if got := mgr.ClusterCount(); got != 2 {
		t.Fatalf("ClusterCount()=%d, expected 2", got)
	}

	mgr.ClearCluster("ns/cluster1")

	if got := mgr.ClusterCount(); got != 1 {
		t.Errorf("ClusterCount()=%d after clear, expected 1", got)
	}

	// Clear again should be a no-op
	mgr.ClearCluster("ns/cluster1")
	if got := mgr.ClusterCount(); got != 1 {
		t.Errorf("ClusterCount()=%d after double clear, expected 1", got)
	}
}

func TestClientManager_Close(t *testing.T) {
	t.Parallel()

	mgr := NewClientManager(ClientConfig{})

	_ = mgr.FactoryFor("ns/cluster1", nil)
	_ = mgr.FactoryFor("ns/cluster2", nil)

	mgr.Close()

	if got := mgr.ClusterCount(); got != 0 {
		t.Errorf("ClusterCount()=%d after Close, expected 0", got)
	}
}

func TestClientManager_NilReceiver(t *testing.T) {
	t.Parallel()

	var mgr *ClientManager

	// These should not panic
	if f := mgr.FactoryFor("ns/cluster", nil); f != nil {
		t.Errorf("expected nil factory for nil manager")
	}

	mgr.Close()
	mgr.ClearCluster("ns/cluster")

	if got := mgr.ClusterCount(); got != 0 {
		t.Errorf("ClusterCount()=%d for nil manager, expected 0", got)
	}
}

func TestClientManager_CircuitBreakerIsolation(t *testing.T) {
	t.Parallel()

	// Create two separate ClientManagers
	mgr1 := NewClientManager(ClientConfig{
		RateLimitQPS:                   1000,
		RateLimitBurst:                 1000,
		CircuitBreakerFailureThreshold: 2,
		CircuitBreakerOpenDuration:     30 * time.Second,
	})
	defer mgr1.Close()

	mgr2 := NewClientManager(ClientConfig{
		RateLimitQPS:                   1000,
		RateLimitBurst:                 1000,
		CircuitBreakerFailureThreshold: 2,
		CircuitBreakerOpenDuration:     30 * time.Second,
	})
	defer mgr2.Close()

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != constants.APIPathSysStepDown {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	// Use the same clusterKey with different managers
	clusterKey := "test/cluster"

	f1 := mgr1.FactoryFor(clusterKey, nil)
	c1, err := f1.NewWithToken(server.URL, "token")
	if err != nil {
		t.Fatalf("NewWithToken() error: %v", err)
	}

	// Trip the circuit breaker on manager 1's client
	_ = c1.StepDown(context.Background())
	_ = c1.StepDown(context.Background())

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected 2 requests before circuit open, got %d", got)
	}

	// Circuit on mgr1 should be open
	err = c1.StepDown(context.Background())
	if err == nil || !operatorerrors.IsTransientRemoteOverloaded(err) {
		t.Fatalf("expected circuit breaker to block request, got %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected circuit breaker to block without new request; got %d requests", got)
	}

	// But manager 2's client should have its own circuit (not tripped)
	f2 := mgr2.FactoryFor(clusterKey, nil)
	c2, err := f2.NewWithToken(server.URL, "token")
	if err != nil {
		t.Fatalf("NewWithToken() error: %v", err)
	}

	// This should hit the server (different manager, different state)
	_ = c2.StepDown(context.Background())
	if got := atomic.LoadInt32(&requests); got != 3 {
		t.Fatalf("expected 3 requests (manager 2 not blocked), got %d", got)
	}
}

func TestClientManager_FactoryCreatesWorkingClients(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == constants.APIPathSysHealth {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"initialized":true,"sealed":false,"standby":false}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	mgr := NewClientManager(ClientConfig{})
	defer mgr.Close()

	factory := mgr.FactoryFor("test/cluster", nil)
	client, err := factory.New(server.URL)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}

	if !health.Initialized {
		t.Errorf("expected Initialized=true")
	}
	if health.Sealed {
		t.Errorf("expected Sealed=false")
	}
}
