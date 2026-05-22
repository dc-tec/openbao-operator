package raftops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

const (
	testUpgradeInlineAuthPathHeader          = "X-Vault-Inline-Auth-Path"
	testUpgradeInlineAuthOperationHeader     = "X-Vault-Inline-Auth-Operation"
	testUpgradeInlineAuthRoleParameterHeader = "X-Vault-Inline-Auth-Parameter-role"
	testUpgradeInlineAuthJWTParameterHeader  = "X-Vault-Inline-Auth-Parameter-jwt"
	testUpgradeVaultTokenHeader              = "X-Vault-Token"

	testUpgradeInlineAuthJWTPath   = "jwt-operator/login"
	testUpgradeInlineAuthOperation = "update"
	testUpgradeJWTLoginPath        = "/v1/auth/jwt-operator/login"
	testUpgradeStepDownPath        = "/v1/sys/step-down"
	testUpgradeJWTAuthRole         = "upgrade-role"
	testUpgradeJWTToken            = "upgrade-jwt"
	testUpgradeStandardAuthToken   = "upgrade-token"
)

type testUpgradeInlineAuthParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func TestNewAuthenticatedClient_DefaultInlineUsesInlineAuthHeaders(t *testing.T) {
	t.Parallel()

	var loginRequests int32
	var stepDownRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testUpgradeJWTLoginPath:
			atomic.AddInt32(&loginRequests, 1)
			http.Error(w, "unexpected login", http.StatusInternalServerError)
		case testUpgradeStepDownPath:
			atomic.AddInt32(&stepDownRequests, 1)
			if r.Method != http.MethodPut {
				t.Errorf("step-down method=%s, want PUT", r.Method)
			}
			requireUpgradeInlineAuthHeaders(t, r, testUpgradeJWTAuthRole, testUpgradeJWTToken)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	factory, cleanup, err := NewOpenBaoClientFactory(newJWTUpgradeConfig(""))
	if err != nil {
		t.Fatalf("NewOpenBaoClientFactory() error: %v", err)
	}
	defer cleanup()

	client, err := NewAuthenticatedClient(context.Background(), newJWTUpgradeConfig(""), factory, server.URL)
	if err != nil {
		t.Fatalf("NewAuthenticatedClient() error: %v", err)
	}
	if err := client.StepDown(context.Background()); err != nil {
		t.Fatalf("StepDown() error: %v", err)
	}

	if got := atomic.LoadInt32(&loginRequests); got != 0 {
		t.Fatalf("login requests=%d, want 0", got)
	}
	if got := atomic.LoadInt32(&stepDownRequests); got != 1 {
		t.Fatalf("step-down requests=%d, want 1", got)
	}
}

func TestNewAuthenticatedClient_StandardStrategyUsesClientToken(t *testing.T) {
	t.Parallel()

	var loginRequests int32
	var stepDownRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testUpgradeJWTLoginPath:
			atomic.AddInt32(&loginRequests, 1)
			if r.Method != http.MethodPost {
				t.Errorf("login method=%s, want POST", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"auth":{"client_token":"` + testUpgradeStandardAuthToken + `"}}`))
		case testUpgradeStepDownPath:
			atomic.AddInt32(&stepDownRequests, 1)
			if got := r.Header.Get(testUpgradeVaultTokenHeader); got != testUpgradeStandardAuthToken {
				t.Errorf("%s=%q, want %q", testUpgradeVaultTokenHeader, got, testUpgradeStandardAuthToken)
			}
			if got := r.Header.Get(testUpgradeInlineAuthPathHeader); got != "" {
				t.Errorf("%s=%q, want empty", testUpgradeInlineAuthPathHeader, got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := newJWTUpgradeConfig(portopenbao.JWTAuthStrategyStandard)
	factory, cleanup, err := NewOpenBaoClientFactory(cfg)
	if err != nil {
		t.Fatalf("NewOpenBaoClientFactory() error: %v", err)
	}
	defer cleanup()

	client, err := NewAuthenticatedClient(context.Background(), cfg, factory, server.URL)
	if err != nil {
		t.Fatalf("NewAuthenticatedClient() error: %v", err)
	}
	if err := client.StepDown(context.Background()); err != nil {
		t.Fatalf("StepDown() error: %v", err)
	}

	if got := atomic.LoadInt32(&loginRequests); got != 1 {
		t.Fatalf("login requests=%d, want 1", got)
	}
	if got := atomic.LoadInt32(&stepDownRequests); got != 1 {
		t.Fatalf("step-down requests=%d, want 1", got)
	}
}

func newJWTUpgradeConfig(strategy string) *ExecutorConfig {
	return &ExecutorConfig{
		ClusterNamespace: "default",
		ClusterName:      "openbao",
		JWTAuthRole:      testUpgradeJWTAuthRole,
		JWTAuthStrategy:  strategy,
		JWTToken:         testUpgradeJWTToken,
	}
}

func requireUpgradeInlineAuthHeaders(t *testing.T, r *http.Request, role, jwtToken string) {
	t.Helper()

	if got := r.Header.Get(testUpgradeVaultTokenHeader); got != "" {
		t.Fatalf("%s=%q, want empty", testUpgradeVaultTokenHeader, got)
	}
	if got := r.Header.Get(testUpgradeInlineAuthPathHeader); got != testUpgradeInlineAuthJWTPath {
		t.Fatalf("%s=%q, want %q", testUpgradeInlineAuthPathHeader, got, testUpgradeInlineAuthJWTPath)
	}
	if got := r.Header.Get(testUpgradeInlineAuthOperationHeader); got != testUpgradeInlineAuthOperation {
		t.Fatalf("%s=%q, want %q", testUpgradeInlineAuthOperationHeader, got, testUpgradeInlineAuthOperation)
	}

	requireUpgradeInlineAuthParameter(t, r.Header.Get(testUpgradeInlineAuthRoleParameterHeader), "role", role)
	requireUpgradeInlineAuthParameter(t, r.Header.Get(testUpgradeInlineAuthJWTParameterHeader), "jwt", jwtToken)
}

func requireUpgradeInlineAuthParameter(t *testing.T, encoded, key, value string) {
	t.Helper()

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode inline auth parameter %q: %v", key, err)
	}
	var param testUpgradeInlineAuthParameter
	if err := json.Unmarshal(decoded, &param); err != nil {
		t.Fatalf("failed to unmarshal inline auth parameter %q: %v", key, err)
	}
	if param.Key != key || param.Value != value {
		t.Fatalf("inline auth parameter=%#v, want key=%q value=%q", param, key, value)
	}
}
