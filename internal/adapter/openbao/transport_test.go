package openbao

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestSmartClient_CircuitBreaker_SharedAcrossClients(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != apiPathSysStepDown {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	// Use ClientManager for shared state
	mgr := NewClientManager(ClientConfig{
		RateLimitQPS:                   1000,
		RateLimitBurst:                 1000,
		CircuitBreakerFailureThreshold: 2,
		CircuitBreakerOpenDuration:     30 * time.Second,
	})
	defer mgr.Close()

	factory := mgr.FactoryFor("tenant-a/cluster-a", nil)

	c1, err := factory.NewWithToken(server.URL, "s.valid-token")
	if err != nil {
		t.Fatalf("NewWithToken() error: %v", err)
	}
	c2, err := factory.NewWithToken(server.URL, "s.valid-token")
	if err != nil {
		t.Fatalf("NewWithToken() error: %v", err)
	}

	// Two overload failures should open the circuit.
	if err := c1.StepDown(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
	if err := c1.StepDown(context.Background()); err == nil {
		t.Fatalf("expected error")
	}

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected 2 requests before circuit open, got %d", got)
	}

	// Third attempt (via a different Client instance from matching factory) should be blocked without hitting the server.
	err = c2.StepDown(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if !operatorerrors.IsTransientRemoteOverloaded(err) {
		t.Fatalf("expected transient remote overloaded error, got %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected circuit breaker to block without new request; got %d requests", got)
	}
}

func TestClient_DoRequestRecordsRequestMetric(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"leader_address":"https://openbao-0.openbao:8200"}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	req, err := client.newRequest(context.Background(), http.MethodGet, apiPathSysLeader, nil)
	if err != nil {
		t.Fatalf("newRequest() error: %v", err)
	}

	counter := clientRequestsTotal.WithLabelValues(http.MethodGet, apiPathSysLeader, "200", "success")
	before := testutil.ToFloat64(counter)
	resp, err := client.doRequest(req, nil, "test request")
	if err != nil {
		t.Fatalf("doRequest() error: %v", err)
	}
	drainAndClose(resp)
	after := testutil.ToFloat64(counter)
	if after != before+1 {
		t.Fatalf("request counter delta = %v, want 1", after-before)
	}
}
