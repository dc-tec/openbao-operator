package openbao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestClientFactory_New_NilReceiver(t *testing.T) {
	var factory *ClientFactory

	if _, err := factory.New("http://example"); err == nil {
		t.Fatalf("expected error for nil factory")
	}
}

func TestClientFactory_NewWithToken_UsesProvidedValues(t *testing.T) {
	factory := NewClientFactory(ClientConfig{
		BaseURL: "http://should-not-be-used",
		Token:   "should-not-be-used",
	})

	client, err := factory.NewWithToken("http://example", "s.token")
	if err != nil {
		t.Fatalf("NewWithToken() error: %v", err)
	}

	if got := client.BaseURL(); got != "http://example" {
		t.Fatalf("BaseURL()=%q, expected %q", got, "http://example")
	}
	if got := client.Token(); got != "s.token" {
		t.Fatalf("Token()=%q, expected %q", got, "s.token")
	}
}

func TestClientFactory_New_InvalidCACert(t *testing.T) {
	factory := NewClientFactory(ClientConfig{
		CACert: []byte("not a cert"),
	})

	if _, err := factory.New("http://example"); err == nil {
		t.Fatalf("expected error for invalid CA cert")
	}
}

func TestClientFactory_LoginJWT(t *testing.T) {
	t.Parallel()

	type requestBody struct {
		Role string `json:"role"`
		JWT  string `json:"jwt"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != constants.APIPathAuthJWTLogin {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type=%q, expected application/json", got)
		}

		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body.Role != "role" || body.JWT != "jwt" {
			t.Fatalf("unexpected body: %#v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"token"}}`))
	}))
	defer server.Close()

	factory := NewClientFactory(ClientConfig{})

	token, err := factory.LoginJWT(context.Background(), server.URL, "role", "jwt")
	if err != nil {
		t.Fatalf("LoginJWT() error: %v", err)
	}
	if token != "token" {
		t.Fatalf("LoginJWT()=%q, expected %q", token, "token")
	}
}

func TestClientFactory_NewWithJWT(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPathAuthJWTLogin {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"token"}}`))
	}))
	defer server.Close()

	factory := NewClientFactory(ClientConfig{})
	client, err := factory.NewWithJWT(context.Background(), server.URL, "role", "jwt")
	if err != nil {
		t.Fatalf("NewWithJWT() error: %v", err)
	}
	if got := client.Token(); got != "token" {
		t.Fatalf("Token()=%q, expected %q", got, "token")
	}
}

func TestClientFactory_New_ReusesHTTPClient(t *testing.T) {
	factory := NewClientFactory(ClientConfig{})
	url1 := "http://example.com/1"
	url2 := "http://example.com/2"

	// Create first client
	client1, err := factory.New(url1)
	if err != nil {
		t.Fatalf("New(url1) error: %v", err)
	}

	// Create second client for SAME url
	client2, err := factory.New(url1)
	if err != nil {
		t.Fatalf("New(url1) again error: %v", err)
	}

	// Check underlying HTTP client identity (pointer comparison)
	// We access the unexported field via reflection or assuming we can't...
	// Wait, we can't access unexported fields in tests unless in same package.
	// We ARE in package openbao.
	if client1.httpClient != client2.httpClient {
		t.Errorf("New(url1) did not reuse http.Client; got %p and %p", client1.httpClient, client2.httpClient)
	}

	// Create third client for DIFFERENT url
	client3, err := factory.New(url2)
	if err != nil {
		t.Fatalf("New(url2) error: %v", err)
	}

	if client1.httpClient == client3.httpClient {
		t.Errorf("New(url2) reused http.Client from url1; got %p", client1.httpClient)
	}
}

func TestClientFactory_LoginJWT_Caching(t *testing.T) {
	t.Parallel()

	var loginRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == constants.APIPathAuthJWTLogin {
			atomic.AddInt32(&loginRequests, 1)
			w.Header().Set("Content-Type", "application/json")
			// Return a token with 12s TTL (2s valid cache, given 10s buffer)
			_, _ = w.Write([]byte(`{"auth":{"client_token":"cached-token","ttl":12}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	factory := NewClientFactory(ClientConfig{})

	// First login - hits server
	token, err := factory.LoginJWT(context.Background(), server.URL, "role", "jwt")
	if err != nil {
		t.Fatalf("LoginJWT error: %v", err)
	}
	if token != "cached-token" {
		t.Errorf("unexpected token: %s", token)
	}
	if count := atomic.LoadInt32(&loginRequests); count != 1 {
		t.Errorf("expected 1 login request, got %d", count)
	}

	// Second login immediately - uses cache
	token, err = factory.LoginJWT(context.Background(), server.URL, "role", "jwt")
	if err != nil {
		t.Fatalf("LoginJWT 2 error: %v", err)
	}
	if token != "cached-token" {
		t.Errorf("unexpected token 2: %s", token)
	}
	if count := atomic.LoadInt32(&loginRequests); count != 1 {
		t.Errorf("expected 1 login request, got %d (cache not used)", count)
	}

	// Wait for cache to expire (TTL 12s - 10s buffer = 2s valid). Wait 2.5s.
	time.Sleep(2500 * time.Millisecond)

	// Third login - expires, hits server
	_, err = factory.LoginJWT(context.Background(), server.URL, "role", "jwt")
	if err != nil {
		t.Fatalf("LoginJWT 3 error: %v", err)
	}
	if count := atomic.LoadInt32(&loginRequests); count != 2 {
		t.Errorf("expected 2 login requests, got %d (cache not expired?)", count)
	}
}
