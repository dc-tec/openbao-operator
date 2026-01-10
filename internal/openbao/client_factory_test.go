package openbao

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
