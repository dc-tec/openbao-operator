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

	"k8s.io/utils/ptr"
)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != healthPath {
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
	tests := []struct {
		name       string
		response   HealthResponse
		wantLeader bool
		wantErr    bool
	}{
		{
			name: "active leader",
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     false,
			},
			wantLeader: true,
			wantErr:    false,
		},
		{
			name: "standby",
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
				Standby:     true,
			},
			wantLeader: false,
			wantErr:    false,
		},
		{
			name: "sealed",
			response: HealthResponse{
				Initialized: true,
				Sealed:      true,
				Standby:     false,
			},
			wantLeader: false,
			wantErr:    false,
		},
		{
			name: "not initialized",
			response: HealthResponse{
				Initialized: false,
				Sealed:      true,
				Standby:     false,
			},
			wantLeader: false,
			wantErr:    false,
		},
		{
			name: "performance standby",
			response: HealthResponse{
				Initialized:        true,
				Sealed:             false,
				Standby:            false,
				PerformanceStandby: true,
			},
			wantLeader: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewEncoder(w).Encode(tt.response); err != nil {
					t.Fatal(err)
				}
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			isLeader, err := client.IsLeader(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("IsLeader() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if isLeader != tt.wantLeader {
				t.Errorf("IsLeader() = %v, want %v", isLeader, tt.wantLeader)
			}
		})
	}
}

func TestClient_StepDown(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		statusCode int
		wantErr    bool
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
		},
		{
			name:       "internal error",
			token:      "s.valid-token",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != stepDownPath {
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
			}
		})
	}
}

func TestClient_IsHealthy(t *testing.T) {
	tests := []struct {
		name        string
		response    HealthResponse
		wantHealthy bool
		wantErr     bool
	}{
		{
			name: "healthy - initialized and unsealed",
			response: HealthResponse{
				Initialized: true,
				Sealed:      false,
			},
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "unhealthy - sealed",
			response: HealthResponse{
				Initialized: true,
				Sealed:      true,
			},
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "unhealthy - not initialized",
			response: HealthResponse{
				Initialized: false,
				Sealed:      true,
			},
			wantHealthy: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewEncoder(w).Encode(tt.response); err != nil {
					t.Fatal(err)
				}
			}))
			defer server.Close()

			client, err := NewClient(ClientConfig{BaseURL: server.URL})
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			healthy, err := client.IsHealthy(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("IsHealthy() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if healthy != tt.wantHealthy {
				t.Errorf("IsHealthy() = %v, want %v", healthy, tt.wantHealthy)
			}
		})
	}
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
				if r.URL.Path != initPath {
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
				if tt.wantErrMessage != "" && !errors.Is(err, nil) && !containsError(err, tt.wantErrMessage) {
					t.Fatalf("Init() error = %v, want message containing %q", err, tt.wantErrMessage)
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

func containsError(err error, substr string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substr)
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
				if r.URL.Path != jwtAuthPath {
					t.Errorf("unexpected path: %s, want %s", r.URL.Path, jwtAuthPath)
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

			token, err := client.LoginJWT(context.Background(), tt.role, tt.jwtToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoginJWT() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.wantErrMessage != "" && !containsError(err, tt.wantErrMessage) {
					t.Errorf("LoginJWT() error = %v, want message containing %q", err, tt.wantErrMessage)
				}
				return
			}

			if token != tt.wantToken {
				t.Errorf("LoginJWT() token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}
