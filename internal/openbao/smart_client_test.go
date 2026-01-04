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

func TestSmartClient_CircuitBreaker_SharedAcrossClients(t *testing.T) {
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

	cfg := ClientConfig{
		ClusterKey:                     "tenant-a/cluster-a",
		BaseURL:                        server.URL,
		Token:                          "s.valid-token",
		RateLimitQPS:                   1000,
		RateLimitBurst:                 1000,
		CircuitBreakerFailureThreshold: 2,
		CircuitBreakerOpenDuration:     30 * time.Second,
	}

	c1, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	c2, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
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

	// Third attempt (via a different Client instance) should be blocked without hitting the server.
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
