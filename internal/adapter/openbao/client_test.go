package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"k8s.io/utils/ptr"
)

type healthBoolTestCase struct {
	name     string
	response HealthResponse
	want     bool
	wantErr  bool
}

func newHealthResponseClient(t *testing.T, response HealthResponse) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client
}

func runHealthBoolTests(
	t *testing.T,
	tests []healthBoolTestCase,
	methodName string,
	call func(context.Context, *Client) (bool, error),
) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newHealthResponseClient(t, tt.response)

			got, err := call(context.Background(), client)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s() error = %v, wantErr %v", methodName, err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("%s() = %v, want %v", methodName, got, tt.want)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  ClientConfig
		wantErr bool
	}{
		{
			name: "valid config with URL only",
			config: ClientConfig{
				BaseURL: "https://localhost:8200",
			},
			wantErr: false,
		},
		{
			name: "valid config with token",
			config: ClientConfig{
				BaseURL: "https://localhost:8200",
				Token:   "s.abcdef123456",
			},
			wantErr: false,
		},
		{
			name: "valid config with empty CA cert uses system pool",
			config: ClientConfig{
				BaseURL: "https://localhost:8200",
				CACert:  []byte{}, // Empty CA cert should use system pool
			},
			wantErr: false,
		},
		{
			name:    "empty URL",
			config:  ClientConfig{},
			wantErr: true,
		},
		{
			name: "invalid CA cert",
			config: ClientConfig{
				BaseURL: "https://localhost:8200",
				CACert:  []byte("not a valid cert"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_Health(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		response       HealthResponse
		wantErr        bool
		wantInitalized bool
		wantSealed     bool
		wantStandby    bool
	}{
		{
			name:       "active and healthy",
			statusCode: http.StatusOK,
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     false,
				Version:     "2.4.0",
				ClusterName: "test-cluster",
			},
			wantErr:        false,
			wantInitalized: true,
			wantSealed:     false,
			wantStandby:    false,
		},
		{
			name:       "standby node",
			statusCode: http.StatusTooManyRequests, // 429
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     true,
				Version:     "2.4.0",
			},
			wantErr:        false,
			wantInitalized: true,
			wantSealed:     false,
			wantStandby:    true,
		},
		{
			name:       "sealed node",
			statusCode: http.StatusServiceUnavailable, // 503
			response: HealthResponse{
				Initialized: true,
				Sealed:      true,
				Standby:     false,
			},
			wantErr:        false,
			wantInitalized: true,
			wantSealed:     true,
			wantStandby:    false,
		},
		{
			name:       "not initialized",
			statusCode: http.StatusNotImplemented, // 501
			response: HealthResponse{
				Initialized: false,
				Sealed:      true,
				Standby:     false,
			},
			wantErr:        false,
			wantInitalized: false,
			wantSealed:     true,
			wantStandby:    false,
		},
		{
			name:       "performance standby",
			statusCode: 473,
			response: HealthResponse{
				Initialized:        true,
				Sealed:             false,
				Standby:            false,
				PerformanceStandby: true,
			},
			wantErr:        false,
			wantInitalized: true,
			wantSealed:     false,
			wantStandby:    false,
		},
		{
			name:       "forbidden (safe mode)",
			statusCode: http.StatusForbidden,
			response:   HealthResponse{
				// The body is usually an error struct, but here we just simulate
				// that we don't get a valid health response.
				// We expect an error, NOT a zero-value struct.
			},
			wantErr:        true,
			wantInitalized: false,
			wantSealed:     false, // correctly uninitialized/empty struct would have false here
			wantStandby:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != constants.APIPathSysHealth {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method: %s", r.Method)
				}

				w.WriteHeader(tt.statusCode)
				if err := json.NewEncoder(w).Encode(tt.response); err != nil {
					t.Fatal(err)
				}
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			health, err := client.Health(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Health() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if health.Initialized != tt.wantInitalized {
					t.Errorf("Health().Initialized = %v, want %v", health.Initialized, tt.wantInitalized)
				}
				if health.Sealed != tt.wantSealed {
					t.Errorf("Health().Sealed = %v, want %v", health.Sealed, tt.wantSealed)
				}
				if health.Standby != tt.wantStandby {
					t.Errorf("Health().Standby = %v, want %v", health.Standby, tt.wantStandby)
				}
			}
		})
	}
}

func TestClient_IsLeader(t *testing.T) {
	tests := []healthBoolTestCase{
		{
			name: "active leader",
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     false,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "standby",
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     true,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "sealed",
			response: HealthResponse{
				Initialized: true,
				Sealed:      true,
				Standby:     false,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "not initialized",
			response: HealthResponse{
				Initialized: false,
				Sealed:      true,
				Standby:     false,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "performance standby",
			response: HealthResponse{
				Initialized:        true,
				Sealed:             false,
				Standby:            false,
				PerformanceStandby: true,
			},
			want:    false,
			wantErr: false,
		},
	}

	runHealthBoolTests(t, tests, "IsLeader", func(ctx context.Context, client *Client) (bool, error) {
		return client.IsLeader(ctx)
	})
}

func TestClient_StepDown(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		statusCode int
		wantErr    bool
		wantStatus int
	}{
		{
			name:       "successful step-down with 204",
			token:      "s.valid-token",
			statusCode: http.StatusNoContent,
			wantErr:    false,
		},
		{
			name:       "successful step-down with 200",
			token:      "s.valid-token",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "no token",
			token:      "",
			statusCode: http.StatusForbidden,
			wantErr:    true,
		},
		{
			name:       "forbidden",
			token:      "s.invalid-token",
			statusCode: http.StatusForbidden,
			wantErr:    true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "internal error",
			token:      "s.valid-token",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != constants.APIPathSysStepDown {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPut {
					t.Errorf("unexpected method: %s", r.Method)
				}

				// Check for token header
				token := r.Header.Get("X-Vault-Token")
				if tt.token != "" && token != tt.token {
					t.Errorf("unexpected token: got %s, want %s", token, tt.token)
				}

				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   tt.token,
			})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			err = client.StepDown(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("StepDown() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantStatus != 0 {
				assertStatusCode(t, err, tt.wantStatus)
			}
		})
	}
}

func TestClient_JoinRaftCluster_AlreadyJoinedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPathRaftJoin {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":["node already joined to cluster"]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "s.test-token",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.JoinRaftCluster(context.Background(), "https://leader.example:8200", true, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, portopenbao.ErrAlreadyJoined) {
		t.Fatalf("expected ErrAlreadyJoined, got %v", err)
	}
	assertStatusCode(t, err, http.StatusBadRequest)
}

func TestClient_JoinRaftCluster_NotJoinedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPathRaftJoin {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"joined":false}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "s.test-token",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.JoinRaftCluster(context.Background(), "https://leader.example:8200", true, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, portopenbao.ErrAlreadyJoined) {
		t.Fatalf("expected ErrAlreadyJoined, got %v", err)
	}
}

func TestClient_DemoteRaftPeer_AlreadyNonVoterStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPathRaftDemotePeer {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":["peer is already a non-voter"]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "s.test-token",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = client.DemoteRaftPeer(context.Background(), "node-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, portopenbao.ErrAlreadyNonVoter) {
		t.Fatalf("expected ErrAlreadyNonVoter, got %v", err)
	}
	assertStatusCode(t, err, http.StatusBadRequest)
}

func TestClient_IsHealthy(t *testing.T) {
	tests := []healthBoolTestCase{
		{
			name: "healthy - initialized and unsealed",
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "unhealthy - sealed",
			response: HealthResponse{
				Initialized: true,
				Sealed:      true,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "unhealthy - not initialized",
			response: HealthResponse{
				Initialized: false,
				Sealed:      true,
			},
			want:    false,
			wantErr: false,
		},
	}

	runHealthBoolTests(t, tests, "IsHealthy", func(ctx context.Context, client *Client) (bool, error) {
		return client.IsHealthy(ctx)
	})
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL:        server.URL,
		RequestTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.Health(ctx)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestClient_NetworkError(t *testing.T) {
	// Use a URL that won't connect
	client, err := NewClient(ClientConfig{
		BaseURL:           "http://localhost:9999",
		ConnectionTimeout: 100 * time.Millisecond,
		RequestTimeout:    100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = client.Health(ctx)
	if err == nil {
		t.Error("expected network error")
	}
}

func TestClient_Init(t *testing.T) {
	tests := []struct {
		name           string
		request        InitRequest
		statusCode     int
		responseBody   interface{}
		wantErr        bool
		wantToken      string
		wantShares     int
		wantThreshold  int
		wantErrMessage string
		wantStatusCode int
		wantErrIs      error
	}{
		{
			name: "successful init",
			request: InitRequest{
				SecretShares:    ptr.To(1),
				SecretThreshold: ptr.To(1),
			},
			statusCode: http.StatusOK,
			responseBody: InitResponse{
				UnsealKeysB64: []string{"key1"},
				RootToken:     "s.root-token",
			},
			wantErr:       false,
			wantToken:     "s.root-token",
			wantShares:    1,
			wantThreshold: 1,
		},
		{
			name: "already initialized error",
			request: InitRequest{
				SecretShares:    ptr.To(1),
				SecretThreshold: ptr.To(1),
			},
			statusCode:     http.StatusBadRequest,
			responseBody:   map[string]string{"error": "already initialized"},
			wantErr:        true,
			wantErrMessage: "already initialized",
			wantStatusCode: http.StatusBadRequest,
			wantErrIs:      portopenbao.ErrAlreadyInitialized,
		},
		{
			name: "invalid shares",
			request: InitRequest{
				SecretShares:    ptr.To(0),
				SecretThreshold: ptr.To(1),
			},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		{
			name: "invalid threshold",
			request: InitRequest{
				SecretShares:    ptr.To(1),
				SecretThreshold: ptr.To(0),
			},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		{
			name: "threshold greater than shares",
			request: InitRequest{
				SecretShares:    ptr.To(1),
				SecretThreshold: ptr.To(2),
			},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var serverInitErr error

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != constants.APIPathSysInit {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				if r.Method != http.MethodPut {
					t.Errorf("unexpected method: %s", r.Method)
				}

				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				}

				if tt.responseBody != nil {
					if err := json.NewEncoder(w).Encode(tt.responseBody); err != nil {
						serverInitErr = err
					}
				}
			}))
			defer server.Close()

			if serverInitErr != nil {
				t.Fatalf("failed to set up test server: %v", serverInitErr)
			}

			client, err := NewClient(ClientConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			resp, err := client.Init(context.Background(), tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.wantErrMessage != "" && !containsError(err, tt.wantErrMessage) {
					t.Fatalf("Init() error = %v, want message containing %q", err, tt.wantErrMessage)
				}
				if tt.wantStatusCode != 0 {
					assertStatusCode(t, err, tt.wantStatusCode)
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("Init() error = %v, want errors.Is(..., %v)", err, tt.wantErrIs)
				}
				return
			}

			if resp == nil {
				t.Fatal("Init() response is nil")
			}

			if resp.RootToken != tt.wantToken {
				t.Errorf("Init().RootToken = %q, want %q", resp.RootToken, tt.wantToken)
			}
		})
	}
}

func TestClient_Init_UsesContextDeadlineBeyondDefaultRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPathSysInit {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Simulate init taking longer than the client's default request timeout.
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(InitResponse{
			UnsealKeysB64: []string{"key1"},
			RootToken:     "s.root-token",
		}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL:        server.URL,
		RequestTimeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	resp, err := client.Init(ctx, InitRequest{
		SecretShares:    ptr.To(1),
		SecretThreshold: ptr.To(1),
	})
	if err != nil {
		t.Fatalf("Init() error = %v, want nil", err)
	}
	if resp == nil {
		t.Fatal("Init() response is nil")
	}
	if resp.RootToken != "s.root-token" {
		t.Fatalf("Init().RootToken = %q, want %q", resp.RootToken, "s.root-token")
	}
}

func containsError(err error, substr string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substr)
}

func assertStatusCode(t *testing.T, err error, want int) {
	t.Helper()

	got, ok := portopenbao.StatusCode(err)
	if !ok {
		t.Fatalf("expected API status %d, got non-API error %v", want, err)
	}
	if got != want {
		t.Fatalf("status code = %d, want %d (err=%v)", got, want, err)
	}
}

func TestClient_LoginJWT(t *testing.T) {
	tests := []struct {
		name           string
		role           string
		jwtToken       string
		statusCode     int
		responseBody   interface{}
		wantErr        bool
		wantToken      string
		wantErrMessage string
		wantStatusCode int
	}{
		{
			name:       "successful authentication",
			role:       "backup-role",
			jwtToken:   "eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.test-token",
			statusCode: http.StatusOK,
			responseBody: JWTAuthLoginResponse{
				Auth: struct {
					ClientToken string `json:"client_token"`
					LeaseID     string `json:"lease_id"`
					Renewable   bool   `json:"renewable"`
					TTL         int    `json:"ttl"`
				}{
					ClientToken: "s.bao-token-12345",
					LeaseID:     "lease-123",
					Renewable:   true,
					TTL:         3600,
				},
			},
			wantErr:   false,
			wantToken: "s.bao-token-12345",
		},
		{
			name:           "empty role",
			role:           "",
			jwtToken:       "test-token",
			wantErr:        true,
			wantErrMessage: "role and jwtToken are required",
		},
		{
			name:           "empty JWT token",
			role:           "backup-role",
			jwtToken:       "",
			wantErr:        true,
			wantErrMessage: "role and jwtToken are required",
		},
		{
			name:       "authentication failed - invalid role",
			role:       "invalid-role",
			jwtToken:   "test-token",
			statusCode: http.StatusForbidden,
			responseBody: map[string]interface{}{
				"errors": []string{"permission denied"},
			},
			wantErr:        true,
			wantErrMessage: "status 403",
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:       "authentication failed - invalid token",
			role:       "backup-role",
			jwtToken:   "invalid-token",
			statusCode: http.StatusBadRequest,
			responseBody: map[string]interface{}{
				"errors": []string{"invalid JWT token"},
			},
			wantErr:        true,
			wantErrMessage: "status 400",
			wantStatusCode: http.StatusBadRequest,
		},
		{
			name:       "missing client_token in response",
			role:       "backup-role",
			jwtToken:   "test-token",
			statusCode: http.StatusOK,
			responseBody: JWTAuthLoginResponse{
				Auth: struct {
					ClientToken string `json:"client_token"`
					LeaseID     string `json:"lease_id"`
					Renewable   bool   `json:"renewable"`
					TTL         int    `json:"ttl"`
				}{
					ClientToken: "",
					LeaseID:     "lease-123",
					Renewable:   true,
					TTL:         3600,
				},
			},
			wantErr:        true,
			wantErrMessage: "missing client_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != constants.APIPathAuthJWTLogin {
					t.Errorf("unexpected path: %s, want %s", r.URL.Path, constants.APIPathAuthJWTLogin)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
				}

				// Verify Content-Type
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("unexpected Content-Type: %s", ct)
				}

				// Parse and verify request body
				var reqBody map[string]string
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Errorf("failed to decode request body: %v", err)
				}
				if reqBody["role"] != tt.role {
					t.Errorf("unexpected role in request: got %s, want %s", reqBody["role"], tt.role)
				}
				if reqBody["jwt"] != tt.jwtToken {
					t.Errorf("unexpected JWT in request: got %s, want %s", reqBody["jwt"], tt.jwtToken)
				}

				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				}

				if tt.responseBody != nil {
					if err := json.NewEncoder(w).Encode(tt.responseBody); err != nil {
						t.Fatal(err)
					}
				}
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			token, _, err := client.LoginJWT(context.Background(), tt.role, tt.jwtToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoginJWT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.wantErrMessage != "" && !containsError(err, tt.wantErrMessage) {
					t.Errorf("LoginJWT() error = %v, want message containing %q", err, tt.wantErrMessage)
				}
				if tt.wantStatusCode != 0 {
					assertStatusCode(t, err, tt.wantStatusCode)
				}
				return
			}

			if token != tt.wantToken {
				t.Errorf("LoginJWT() token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}

func TestClient_ReadRaftAutopilotState_NotFoundPreservesStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPathRaftAutopilotState {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["autopilot not enabled"]}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: server.URL,
		Token:   "s.test-token",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.ReadRaftAutopilotState(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAutopilotNotAvailable) {
		t.Fatalf("expected ErrAutopilotNotAvailable, got %v", err)
	}
	assertStatusCode(t, err, http.StatusNotFound)
}
