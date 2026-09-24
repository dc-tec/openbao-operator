package openbao

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/stretchr/testify/require"
)

const policyTestToken = "test-token"

func TestReadACLPolicy(t *testing.T) {
	const policy = "path \"sys/health\" { capabilities = [\"read\"] }\n"
	stored, err := json.Marshal(map[string]map[string]string{"data": {"policy": policy}})
	require.NoError(t, err)
	for _, tt := range []struct {
		name      string
		status    int
		body      string
		want      *string
		wantError bool
	}{
		{name: "exact contents", status: http.StatusOK, body: string(stored), want: new(policy)},
		{name: "empty policy", status: http.StatusOK, body: `{"data":{"policy":""}}`, want: new("")},
		{name: "missing policy", status: http.StatusNotFound, body: `{"errors":[]}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{"errors":["denied"]}`, wantError: true},
		{name: "server error", status: http.StatusInternalServerError, body: `{"errors":["unavailable"]}`, wantError: true},
		{name: "invalid JSON", status: http.StatusOK, body: `{`, wantError: true},
		{name: "missing data", status: http.StatusOK, body: `{}`, wantError: true},
		{name: "missing contents", status: http.StatusOK, body: `{"data":{}}`, wantError: true},
		{name: "null contents", status: http.StatusOK, body: `{"data":{"policy":null}}`, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handlerErrors := newHTTPHandlerErrors(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/sys/policies/acl/openbao-operator" || r.Header.Get("X-Vault-Token") != policyTestToken || r.URL.RawQuery != "" {
					handlerErrors.Errorf("unexpected policy read request")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client, err := NewClient(ClientConfig{BaseURL: server.URL, Token: policyTestToken})
			require.NoError(t, err)
			actual, err := client.ReadACLPolicy(t.Context(), "openbao-operator")
			if tt.wantError {
				require.Error(t, err)
				require.Nil(t, actual)
				if tt.status != http.StatusOK {
					status, ok := portopenbao.StatusCode(err)
					require.True(t, ok)
					require.Equal(t, tt.status, status)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, actual)
		})
	}
}

func TestWriteACLPolicy(t *testing.T) {
	const policy = "path \"sys/health\" { capabilities = [\"read\"] }\n"
	for _, status := range []int{http.StatusNoContent, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handlerErrors := newHTTPHandlerErrors(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/v1/sys/policies/acl/openbao-operator" || r.Header.Get("X-Vault-Token") != policyTestToken {
					handlerErrors.Errorf("unexpected policy request method, path, or authentication")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 || body["policy"] != policy {
					handlerErrors.Errorf("policy body must preserve exact contents and contain only policy")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			client, err := NewClient(ClientConfig{BaseURL: server.URL, Token: policyTestToken})
			require.NoError(t, err)
			err = client.WriteACLPolicy(t.Context(), "openbao-operator", policy)
			if status == http.StatusForbidden {
				actual, ok := portopenbao.StatusCode(err)
				require.True(t, ok)
				require.Equal(t, status, actual)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
