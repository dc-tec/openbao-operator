package openbao

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/stretchr/testify/require"
)

func TestWriteACLPolicy(t *testing.T) {
	const policy = "path \"sys/health\" { capabilities = [\"read\"] }\n"
	for _, status := range []int{http.StatusNoContent, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handlerErrors := newHTTPHandlerErrors(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/v1/sys/policies/acl/openbao-operator" || r.Header.Get("X-Vault-Token") != "test-token" {
					handlerErrors.Errorf("unexpected policy request method, path, or authentication")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 || body["policy"] != policy {
					handlerErrors.Errorf("policy body must preserve exact contents and contain only policy")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			client, err := NewClient(ClientConfig{BaseURL: server.URL, Token: "test-token"})
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
