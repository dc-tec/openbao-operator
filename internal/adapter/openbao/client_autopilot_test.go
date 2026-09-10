package openbao

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientReadRaftAutopilotConfig(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    int
		body      string
		wantError string
	}{
		{"configuration", http.StatusOK, `{"data":{"cleanup_dead_servers":true,"min_quorum":3,"max_trailing_logs":1000,"last_contact_threshold":"10s","server_stabilization_time":"10s","dead_server_last_contact_threshold":"5m0s"}}`, ""},
		{"missing data", http.StatusOK, `{}`, "missing data"},
		{"null data", http.StatusOK, `{"data":null}`, "missing data"},
		{"malformed JSON", http.StatusOK, `{`, "failed to parse"},
		{"wrong type", http.StatusOK, `{"data":{"min_quorum":"three"}}`, "failed to parse"},
		{"forbidden", http.StatusForbidden, `{"errors":["permission denied"]}`, "autopilot config read request failed"},
		{"not found", http.StatusNotFound, `{}`, "autopilot config read request failed"},
		{"sealed", http.StatusServiceUnavailable, `{}`, "transient remote overloaded"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handlerErrors := newHTTPHandlerErrors(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != apiPathRaftAutopilotConfig || r.Header.Get("X-Vault-Token") != "test-token" {
					handlerErrors.Errorf("unexpected authenticated config read: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			c, err := NewClient(ClientConfig{BaseURL: server.URL, Token: "test-token"})
			require.NoError(t, err)
			config, err := c.ReadRaftAutopilotConfig(t.Context())
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				if tt.status != http.StatusOK && tt.status != http.StatusServiceUnavailable {
					assertStatusCode(t, err, tt.status)
				}
				return
			}
			require.NoError(t, err)
			require.True(t, config.CleanupDeadServers)
			require.Equal(t, 3, config.MinQuorum)
			require.Equal(t, "5m0s", config.DeadServerLastContactThreshold)
		})
	}
}
