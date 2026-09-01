package openbao

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type trackingResponseBody struct {
	reader     io.Reader
	closed     bool
	reachedEOF bool
}

func (b *trackingResponseBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if errors.Is(err, io.EOF) {
		b.reachedEOF = true
	}
	return n, err
}

func (b *trackingResponseBody) Close() error {
	b.closed = true
	return nil
}

type errorThenReader struct {
	err           error
	reader        io.Reader
	returnedError bool
}

func (r *errorThenReader) Read(p []byte) (int, error) {
	if !r.returnedError {
		r.returnedError = true
		return 0, r.err
	}
	return r.reader.Read(p)
}

func TestSmartClient_CircuitBreaker_SharedAcrossClients(t *testing.T) {
	var requests int32
	handlerErrors := newHTTPHandlerErrors(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if r.URL.Path != apiPathSysStepDown {
			handlerErrors.Errorf("unexpected path: %s", r.URL.Path)
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

func TestClient_DoAndReadAllOwnsResponseBody(t *testing.T) {
	readErr := errors.New("response body read failed")
	tests := []struct {
		name           string
		path           string
		statusCode     int
		reader         io.Reader
		wantStatusCode int
		wantBody       string
		wantErr        bool
		wantReadErr    bool
		wantOverloaded bool
		wantDrained    bool
	}{
		{
			name:           "successful response",
			path:           apiPathSysLeader,
			statusCode:     http.StatusOK,
			reader:         strings.NewReader("leader"),
			wantStatusCode: http.StatusOK,
			wantBody:       "leader",
		},
		{
			name:           "overload response",
			path:           apiPathSysLeader,
			statusCode:     http.StatusServiceUnavailable,
			reader:         strings.NewReader("unavailable"),
			wantErr:        true,
			wantOverloaded: true,
		},
		{
			name:       "response body read failure",
			path:       apiPathSysLeader,
			statusCode: http.StatusOK,
			reader: &errorThenReader{
				err:    readErr,
				reader: strings.NewReader("remaining"),
			},
			wantErr:     true,
			wantReadErr: true,
			wantDrained: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseBody := &trackingResponseBody{reader: tt.reader}
			client := &Client{
				httpClient: &http.Client{
					Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: tt.statusCode,
							Header:     make(http.Header),
							Body:       responseBody,
							Request:    req,
						}, nil
					}),
				},
			}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://openbao.example"+tt.path, nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext() error: %v", err)
			}

			gotStatusCode, gotBody, err := client.doAndReadAll(req, nil, "test request")
			if (err != nil) != tt.wantErr {
				t.Fatalf("doAndReadAll() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantReadErr && !errors.Is(err, readErr) {
				t.Errorf("doAndReadAll() error = %v, want wrapped read error", err)
			}
			if tt.wantOverloaded && !operatorerrors.IsTransientRemoteOverloaded(err) {
				t.Errorf("doAndReadAll() error = %v, want transient remote overload", err)
			}
			if gotStatusCode != tt.wantStatusCode {
				t.Errorf("doAndReadAll() status code = %d, want %d", gotStatusCode, tt.wantStatusCode)
			}
			if string(gotBody) != tt.wantBody {
				t.Errorf("doAndReadAll() body = %q, want %q", gotBody, tt.wantBody)
			}
			if tt.wantErr && gotBody != nil {
				t.Errorf("doAndReadAll() body = %q, want nil on error", gotBody)
			}
			if tt.wantDrained && !responseBody.reachedEOF {
				t.Error("doAndReadAll() did not drain response body")
			}
			if !responseBody.closed {
				t.Error("doAndReadAll() did not close response body")
			}
		})
	}
}

func TestInlineJWTAuthorizerRecordsAuthPressureMetric(t *testing.T) {
	auth, err := newInlineJWTAuthorizer("operator-role", "jwt-token")
	if err != nil {
		t.Fatalf("newInlineJWTAuthorizer() error: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sys/leader", nil)

	counter := clientAuthInlineRequestsTotal.WithLabelValues("operator-role")
	before := testutil.ToFloat64(counter)
	if err := auth.authorize(req); err != nil {
		t.Fatalf("authorize() error: %v", err)
	}
	after := testutil.ToFloat64(counter)
	if after != before+1 {
		t.Fatalf("inline auth counter delta = %v, want 1", after-before)
	}
}
